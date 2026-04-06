package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	importTimedLinePattern      = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2}\.\d{3})\s*-\s*(\d{2}:\d{2}:\d{2}\.\d{3})\]\s*(.+)$`)
	importExplicitSpeakerPattern = regexp.MustCompile(`(?i)^speaker\s*([A-Za-z0-9]+)\s*[:：]\s*(.+)$`)
	importSimpleSpeakerPattern   = regexp.MustCompile(`^([A-Za-z0-9一二甲乙男女]+)\s*[:：]\s*(.+)$`)
	importSpeakerTextPattern = regexp.MustCompile(`^(?:(?:speaker|说话人|角色)\s*)?([A-Za-z0-9一二甲乙男女]+)\s*[:：]\s*(.+)$`)
	importReviewOriginalLine = regexp.MustCompile(`^(?:原始转写|原文|原稿|Original)\s*[:：]\s*(.+)$`)
	importReviewReviewedLine = regexp.MustCompile(`^(?:AI\s*校对|校对稿|修订稿|Reviewed)\s*[:：]\s*(.+)$`)
	importReviewIssueLine    = regexp.MustCompile(`^-+\s*\[(.*?)\]\s*(.+)$`)
	importWhitespacePattern  = regexp.MustCompile(`\s+`)
	importAllowedAudioExts   = map[string]bool{".wav": true, ".mp3": true, ".m4a": true, ".flac": true, ".aac": true, ".ogg": true, ".opus": true, ".mp4": true}
	importManagedSourceNames = map[string]bool{"review_manifest.json": true, "review_turns.json": true, "review_turns.txt": true, "chinese_turns.json": true, "chinese_turns.txt": true, "result_manifest.json": true, "english_transcript.json": true, "english_transcript.txt": true, "english_transcript.srt": true}
)

type importSpeakerMapper struct {
	mapping map[string]string
}

type importedEnglishSegment struct {
	Speaker                string  `json:"speaker"`
	Start                  float64 `json:"start"`
	End                    float64 `json:"end"`
	StartTS                string  `json:"start_ts"`
	EndTS                  string  `json:"end_ts"`
	OriginalText           string  `json:"original_text"`
	ReviewedText           string  `json:"reviewed_text"`
	ZHText                 string  `json:"zh_text"`
	EnText                 string  `json:"en_text"`
	SourceTurnIndex        int     `json:"source_turn_index"`
	SegmentIndexWithinTurn int     `json:"segment_index_within_turn"`
}

type importJSONTurn struct {
	TurnIndex    int               `json:"turn_index"`
	Speaker      string            `json:"speaker"`
	Start        float64           `json:"start"`
	End          float64           `json:"end"`
	StartTS      string            `json:"start_ts"`
	EndTS        string            `json:"end_ts"`
	OriginalText string            `json:"original_text"`
	ReviewedText string            `json:"reviewed_text"`
	ZHText       string            `json:"zh_text"`
	Issues       []reviewIssueFile `json:"issues"`
}

func (a *App) ImportChineseTextFile() (JobState, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入中文稿或校对稿",
		Filters: []runtime.FileFilter{{
			DisplayName: "中文文本或 JSON",
			Pattern:     "*.txt;*.md;*.json",
		}},
	})
	if err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetState(), nil
	}
	return a.importChineseSource(path)
}

func (a *App) ImportEnglishTranscriptFile() (JobState, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入英文稿",
		Filters: []runtime.FileFilter{{
			DisplayName: "英文文本或 JSON",
			Pattern:     "*.txt;*.md;*.json;*.srt",
		}},
	})
	if err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(path) == "" {
		return a.GetState(), nil
	}
	return a.importEnglishSource(path)
}

func (a *App) importChineseSource(path string) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前已有任务正在运行")
	}
	currentOutputDir := a.state.OutputDir
	options := a.state.Options
	a.mu.Unlock()

	path = cleanPath(path)
	if path == "" {
		return a.GetState(), errors.New("导入路径为空")
	}
	if _, err := os.Stat(path); err != nil {
		return a.GetState(), fmt.Errorf("导入中文稿失败: %w", err)
	}

	outputDir := chooseImportOutputDir(path, currentOutputDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return a.GetState(), err
	}

	runtimeInfo, err := resolveRuntimePaths()
	if err != nil {
		return a.GetState(), err
	}

	turns, summary, inputAudio, err := loadImportedChineseTurns(path)
	if err != nil {
		return a.GetState(), err
	}

	manifest, err := writeImportedChineseArtifacts(outputDir, inputAudio, turns, summary)
	if err != nil {
		return a.GetState(), err
	}

	options = normalizeOptions(options)
	options.OutputDir = outputDir
	options.TranscriptPath = ""
	if inputAudio != "" {
		options.ReferenceAudioPath = inputAudio
	} else {
		options.ReferenceAudioPath = ""
	}

	a.mu.Lock()
	a.cancelRequested = false
	a.currentCmd = nil
	a.state.Running = false
	a.state.Stage = "review"
	a.state.Status = "done"
	a.state.Message = "已导入中文稿，可继续校对并生成英文稿。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.AudioPath = inputAudio
	a.state.ReferenceAudioPath = options.ReferenceAudioPath
	a.state.OutputDir = outputDir
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.ManifestPath = manifest.Manifest
	a.state.CommandPreview = ""
	a.state.Files = manifest.Files
	a.state.ReviewManifest = manifest
	a.state.Review = ReviewDraft{
		Summary:    summary,
		IssueCount: countIssues(turns),
		Turns:      cloneReviewTurns(turns),
	}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Options = options
	a.state.Runtime = runtimeInfo
	a.state.Logs = nil
	a.appendLogLocked("已导入中文稿来源: " + path)
	a.appendLogLocked("输出目录: " + outputDir)
	a.appendLogLocked(fmt.Sprintf("已识别 %d 段中文文本。", len(turns)))
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()
	return snapshot, nil
}

func (a *App) importEnglishSource(path string) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前已有任务正在运行")
	}
	currentOutputDir := a.state.OutputDir
	options := a.state.Options
	a.mu.Unlock()

	path = cleanPath(path)
	if path == "" {
		return a.GetState(), errors.New("导入路径为空")
	}
	if _, err := os.Stat(path); err != nil {
		return a.GetState(), fmt.Errorf("导入英文稿失败: %w", err)
	}

	outputDir := chooseImportOutputDir(path, currentOutputDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return a.GetState(), err
	}

	runtimeInfo, err := resolveRuntimePaths()
	if err != nil {
		return a.GetState(), err
	}

	segments, reviewTurns, inputAudio, err := loadImportedEnglishSource(path)
	if err != nil {
		return a.GetState(), err
	}

	manifest, err := writeImportedEnglishArtifacts(outputDir, inputAudio, segments, reviewTurns)
	if err != nil {
		return a.GetState(), err
	}

	options = normalizeOptions(options)
	options.OutputDir = outputDir
	options.TranscriptPath = manifest.Files.EnglishTXT
	if inputAudio != "" {
		options.ReferenceAudioPath = inputAudio
	}

	var reviewSummary string
	if len(reviewTurns) > 0 {
		reviewSummary = fmt.Sprintf("已随英文稿一并恢复 %d 段中文校对内容。", len(reviewTurns))
	}

	a.mu.Lock()
	a.cancelRequested = false
	a.currentCmd = nil
	a.state.Running = false
	a.state.Stage = "translate"
	a.state.Status = "done"
	a.state.Message = "已导入英文稿，可直接生成英文音频。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.AudioPath = inputAudio
	if inputAudio != "" {
		a.state.ReferenceAudioPath = inputAudio
	}
	a.state.OutputDir = outputDir
	a.state.EnglishTranscriptPath = manifest.Files.EnglishTXT
	a.state.OutputAudioPath = ""
	a.state.ManifestPath = manifest.Manifest
	a.state.CommandPreview = ""
	a.state.Files = manifest.Files
	a.state.Translation = manifest
	a.state.Options = options
	a.state.Runtime = runtimeInfo
	a.state.Result = SynthesisManifest{}
	if len(reviewTurns) > 0 {
		a.state.Review = ReviewDraft{
			Summary:    reviewSummary,
			IssueCount: countIssues(reviewTurns),
			Turns:      cloneReviewTurns(reviewTurns),
		}
	} else {
		a.state.Review = ReviewDraft{}
		a.state.ReviewManifest = ReviewManifest{}
	}
	a.state.Logs = nil
	a.appendLogLocked("已导入英文稿来源: " + path)
	a.appendLogLocked("输出目录: " + outputDir)
	a.appendLogLocked(fmt.Sprintf("已识别 %d 段英文文本。", len(segments)))
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()
	return snapshot, nil
}

func chooseImportOutputDir(sourcePath string, currentOutputDir string) string {
	if currentOutputDir = cleanPath(currentOutputDir); currentOutputDir != "" {
		return currentOutputDir
	}
	baseName := strings.ToLower(filepath.Base(sourcePath))
	if importManagedSourceNames[baseName] {
		return filepath.Dir(sourcePath)
	}
	return jobOutputDir()
}

func loadImportedChineseTurns(path string) ([]ReviewTurn, string, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	baseName := strings.ToLower(filepath.Base(path))

	if ext == ".json" {
		if baseName == "review_manifest.json" {
			manifest, err := loadReviewManifest(path)
			if err == nil {
				reviewPath := cleanPath(manifest.Files.ReviewJSON)
				if reviewPath == "" {
					reviewPath = filepath.Join(filepath.Dir(path), "review_turns.json")
				}
				turns, loadErr := loadReviewTurns(reviewPath)
				if loadErr == nil && len(turns) > 0 {
					return turns, strings.TrimSpace(manifest.Summary), cleanExistingAudioPath(manifest.InputAudio), nil
				}
			}
		}

		if turns, err := loadReviewTurns(path); err == nil && len(turns) > 0 {
			summary := fmt.Sprintf("已从 %s 导入 %d 段中文校对稿。", filepath.Base(path), len(turns))
			return turns, summary, "", nil
		}

		if turns, err := loadImportedChineseTurnsFromJSON(path); err == nil && len(turns) > 0 {
			summary := fmt.Sprintf("已从 %s 导入 %d 段中文文本。", filepath.Base(path), len(turns))
			return turns, summary, "", nil
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", err
	}
	turns, err := parseChineseTurnsText(string(content))
	if err != nil {
		return nil, "", "", err
	}
	summary := fmt.Sprintf("已导入 %d 段中文文本，可继续校对或直接生成英文稿。", len(turns))
	return turns, summary, "", nil
}

func loadImportedEnglishSource(path string) ([]importedEnglishSegment, []ReviewTurn, string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	baseName := strings.ToLower(filepath.Base(path))

	if ext == ".json" {
		if baseName == "result_manifest.json" {
			manifest, err := loadTranslationManifest(path)
			if err == nil {
				segments, segErr := loadImportedEnglishSegmentsFromManifest(manifest)
				if segErr == nil && len(segments) > 0 {
					var reviewTurns []ReviewTurn
					if reviewPath := cleanPath(manifest.Files.ReviewJSON); reviewPath != "" {
						if loaded, loadErr := loadReviewTurns(reviewPath); loadErr == nil {
							reviewTurns = loaded
						}
					}
					return segments, reviewTurns, cleanExistingAudioPath(manifest.InputAudio), nil
				}
			}
		}

		if segments, err := loadImportedEnglishSegmentsFromJSON(path); err == nil && len(segments) > 0 {
			return segments, nil, "", nil
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	segments, err := parseEnglishSegmentsText(string(content))
	if err != nil {
		return nil, nil, "", err
	}
	return segments, nil, "", nil
}

func loadImportedChineseTurnsFromJSON(path string) ([]ReviewTurn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw []importJSONTurn
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return normalizeImportedChineseTurns(raw)
}

func loadImportedEnglishSegmentsFromManifest(manifest TranslationManifest) ([]importedEnglishSegment, error) {
	if englishJSON := cleanPath(manifest.Files.EnglishJSON); englishJSON != "" {
		if segments, err := loadImportedEnglishSegmentsFromJSON(englishJSON); err == nil && len(segments) > 0 {
			return segments, nil
		}
	}
	englishTXT := cleanPath(manifest.Files.EnglishTXT)
	if englishTXT == "" {
		return nil, errors.New("清单中没有英文稿路径")
	}
	content, err := os.ReadFile(englishTXT)
	if err != nil {
		return nil, err
	}
	return parseEnglishSegmentsText(string(content))
}

func loadImportedEnglishSegmentsFromJSON(path string) ([]importedEnglishSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw []importedEnglishSegment
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return normalizeImportedEnglishSegments(raw)
}

func parseChineseTurnsText(content string) ([]ReviewTurn, error) {
	if turns, err := parseChineseReviewBlocks(content); err == nil && len(turns) > 0 {
		return turns, nil
	}

	rawLines := splitNonEmptyLines(content)
	if len(rawLines) == 0 {
		return nil, errors.New("中文稿内容为空")
	}

	raw := make([]importJSONTurn, 0, len(rawLines))
	for _, line := range rawLines {
		if startTS, endTS, speaker, text, ok := parseTimedSpeakerLine(line); ok {
			raw = append(raw, importJSONTurn{
				Speaker:      speaker,
				StartTS:      startTS,
				EndTS:        endTS,
				OriginalText: text,
				ReviewedText: text,
				ZHText:       text,
			})
			continue
		}
		if speaker, text, ok := parseSpeakerTextLine(line); ok {
			raw = append(raw, importJSONTurn{
				Speaker:      speaker,
				OriginalText: text,
				ReviewedText: text,
				ZHText:       text,
			})
			continue
		}
		raw = append(raw, importJSONTurn{
			OriginalText: line,
			ReviewedText: line,
			ZHText:       line,
		})
	}

	return normalizeImportedChineseTurns(raw)
}

func parseEnglishSegmentsText(content string) ([]importedEnglishSegment, error) {
	lines := splitNonEmptyLines(content)
	if len(lines) == 0 {
		return nil, errors.New("英文稿内容为空")
	}

	raw := make([]importedEnglishSegment, 0, len(lines))
	for _, line := range lines {
		if startTS, endTS, speaker, text, ok := parseTimedSpeakerLine(line); ok {
			raw = append(raw, importedEnglishSegment{
				Speaker: speaker,
				StartTS: startTS,
				EndTS:   endTS,
				EnText:  text,
			})
			continue
		}
		if speaker, text, ok := parseSpeakerTextLine(line); ok {
			raw = append(raw, importedEnglishSegment{
				Speaker: speaker,
				EnText:  text,
			})
			continue
		}
		raw = append(raw, importedEnglishSegment{
			EnText: line,
		})
	}

	return normalizeImportedEnglishSegments(raw)
}

func parseChineseReviewBlocks(content string) ([]ReviewTurn, error) {
	blocks := splitTextBlocks(content)
	if len(blocks) == 0 {
		return nil, errors.New("中文稿内容为空")
	}

	raw := make([]importJSONTurn, 0, len(blocks))
	for _, block := range blocks {
		lines := splitNonEmptyLines(block)
		if len(lines) < 2 {
			continue
		}

		startTS, endTS, speaker, ok := parseTimedSpeakerHeader(lines[0])
		if !ok {
			continue
		}

		var originalText string
		var reviewedText string
		issues := make([]reviewIssueFile, 0)
		for _, line := range lines[1:] {
			switch {
			case importReviewOriginalLine.MatchString(line):
				originalText = strings.TrimSpace(importReviewOriginalLine.FindStringSubmatch(line)[1])
			case importReviewReviewedLine.MatchString(line):
				reviewedText = strings.TrimSpace(importReviewReviewedLine.FindStringSubmatch(line)[1])
			case importReviewIssueLine.MatchString(line):
				match := importReviewIssueLine.FindStringSubmatch(line)
				issues = append(issues, reviewIssueFile{
					Category:   strings.TrimSpace(match[1]),
					Severity:   "medium",
					SourceText: strings.TrimSpace(originalText),
					Suggestion: strings.TrimSpace(reviewedText),
					Reason:     strings.TrimSpace(match[2]),
				})
			}
		}

		if strings.TrimSpace(originalText) == "" && strings.TrimSpace(reviewedText) == "" {
			continue
		}
		raw = append(raw, importJSONTurn{
			Speaker:      speaker,
			StartTS:      startTS,
			EndTS:        endTS,
			OriginalText: originalText,
			ReviewedText: reviewedText,
			Issues:       issues,
		})
	}

	if len(raw) == 0 {
		return nil, errors.New("未识别到校对稿块")
	}
	return normalizeImportedChineseTurns(raw)
}

func normalizeImportedChineseTurns(raw []importJSONTurn) ([]ReviewTurn, error) {
	mapper := importSpeakerMapper{mapping: map[string]string{}}
	turns := make([]ReviewTurn, 0, len(raw))
	cursor := 0.0

	for _, item := range raw {
		originalText := cleanImportText(firstNonEmpty(item.OriginalText, item.ZHText, item.ReviewedText))
		reviewedText := cleanImportText(firstNonEmpty(item.ReviewedText, item.ZHText, item.OriginalText))
		if originalText == "" && reviewedText == "" {
			continue
		}
		if originalText == "" {
			originalText = reviewedText
		}
		if reviewedText == "" {
			reviewedText = originalText
		}

		speaker, err := mapper.resolve(item.Speaker)
		if err != nil {
			return nil, err
		}
		duration := estimateDurationSeconds(reviewedText, false)
		start, end, startTS, endTS := normalizeImportedTiming(item.Start, item.End, item.StartTS, item.EndTS, cursor, duration)
		cursor = end + 0.26

		issues := make([]ReviewIssue, 0, len(item.Issues))
		for _, issue := range item.Issues {
			issues = append(issues, ReviewIssue{
				Category:   strings.TrimSpace(issue.Category),
				Severity:   strings.TrimSpace(issue.Severity),
				SourceText: strings.TrimSpace(issue.SourceText),
				Suggestion: strings.TrimSpace(issue.Suggestion),
				Reason:     strings.TrimSpace(issue.Reason),
			})
		}

		turns = append(turns, ReviewTurn{
			TurnIndex:    len(turns),
			Speaker:      speaker,
			Start:        start,
			End:          end,
			StartTS:      startTS,
			EndTS:        endTS,
			OriginalText: originalText,
			ReviewedText: reviewedText,
			Issues:       issues,
		})
	}

	if len(turns) == 0 {
		return nil, errors.New("未识别到可用的中文段落")
	}
	return turns, nil
}

func normalizeImportedEnglishSegments(raw []importedEnglishSegment) ([]importedEnglishSegment, error) {
	mapper := importSpeakerMapper{mapping: map[string]string{}}
	segments := make([]importedEnglishSegment, 0, len(raw))
	cursor := 0.0

	for _, item := range raw {
		text := cleanImportText(firstNonEmpty(item.EnText, item.ReviewedText, item.OriginalText))
		if text == "" {
			continue
		}

		speaker, err := mapper.resolve(item.Speaker)
		if err != nil {
			return nil, err
		}
		duration := estimateDurationSeconds(text, true)
		start, end, startTS, endTS := normalizeImportedTiming(item.Start, item.End, item.StartTS, item.EndTS, cursor, duration)
		cursor = end + 0.24

		segments = append(segments, importedEnglishSegment{
			Speaker:                speaker,
			Start:                  start,
			End:                    end,
			StartTS:                startTS,
			EndTS:                  endTS,
			OriginalText:           cleanImportText(item.OriginalText),
			ReviewedText:           cleanImportText(item.ReviewedText),
			ZHText:                 cleanImportText(item.ZHText),
			EnText:                 text,
			SourceTurnIndex:        maxInt(item.SourceTurnIndex, len(segments)),
			SegmentIndexWithinTurn: item.SegmentIndexWithinTurn,
		})
	}

	if len(segments) == 0 {
		return nil, errors.New("未识别到可用的英文段落")
	}

	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].Start == segments[j].Start {
			return segments[i].End < segments[j].End
		}
		return segments[i].Start < segments[j].Start
	})
	return segments, nil
}

func writeImportedChineseArtifacts(outputDir string, inputAudio string, turns []ReviewTurn, summary string) (ReviewManifest, error) {
	chineseJSONPath := filepath.Join(outputDir, "chinese_turns.json")
	chineseTXTPath := filepath.Join(outputDir, "chinese_turns.txt")
	reviewJSONPath := filepath.Join(outputDir, "review_turns.json")
	reviewTXTPath := filepath.Join(outputDir, "review_turns.txt")
	manifestPath := filepath.Join(outputDir, "review_manifest.json")

	originalPayload := make([]importJSONTurn, 0, len(turns))
	chineseLines := make([]string, 0, len(turns))
	for _, turn := range turns {
		originalPayload = append(originalPayload, importJSONTurn{
			TurnIndex:    turn.TurnIndex,
			Speaker:      turn.Speaker,
			Start:        turn.Start,
			End:          turn.End,
			StartTS:      turn.StartTS,
			EndTS:        turn.EndTS,
			OriginalText: turn.OriginalText,
			ZHText:       turn.OriginalText,
		})
		chineseLines = append(chineseLines, fmt.Sprintf("[%s - %s] %s: %s", turn.StartTS, turn.EndTS, turn.Speaker, turn.OriginalText))
	}

	if err := writeIndentedJSONFile(chineseJSONPath, originalPayload); err != nil {
		return ReviewManifest{}, err
	}
	if err := os.WriteFile(chineseTXTPath, []byte(strings.Join(chineseLines, "\n")+"\n"), 0o644); err != nil {
		return ReviewManifest{}, err
	}
	if err := saveReviewTurns(reviewJSONPath, turns); err != nil {
		return ReviewManifest{}, err
	}
	if err := os.WriteFile(reviewTXTPath, []byte(renderReviewText(turns)), 0o644); err != nil {
		return ReviewManifest{}, err
	}

	manifest := reviewManifestFile{
		InputAudio:  inputAudio,
		OutputDir:   outputDir,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Turns:       len(turns),
		Issues:      countIssues(turns),
		Summary:     strings.TrimSpace(summary),
		Files: pipelineFiles{
			ChineseJSON: chineseJSONPath,
			ChineseTXT:  chineseTXTPath,
			ReviewJSON:  reviewJSONPath,
			ReviewTXT:   reviewTXTPath,
		},
	}
	if err := writeIndentedJSONFile(manifestPath, manifest); err != nil {
		return ReviewManifest{}, err
	}
	return loadReviewManifest(manifestPath)
}

func writeImportedEnglishArtifacts(outputDir string, inputAudio string, segments []importedEnglishSegment, reviewTurns []ReviewTurn) (TranslationManifest, error) {
	englishJSONPath := filepath.Join(outputDir, "english_transcript.json")
	englishTXTPath := filepath.Join(outputDir, "english_transcript.txt")
	englishSRTPath := filepath.Join(outputDir, "english_transcript.srt")
	manifestPath := filepath.Join(outputDir, "result_manifest.json")

	if err := writeIndentedJSONFile(englishJSONPath, segments); err != nil {
		return TranslationManifest{}, err
	}

	lines := make([]string, 0, len(segments))
	for _, segment := range segments {
		lines = append(lines, fmt.Sprintf("[%s - %s] %s: %s", segment.StartTS, segment.EndTS, segment.Speaker, segment.EnText))
	}
	if err := os.WriteFile(englishTXTPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return TranslationManifest{}, err
	}
	if err := os.WriteFile(englishSRTPath, []byte(renderSRT(segments)), 0o644); err != nil {
		return TranslationManifest{}, err
	}

	files := pipelineFiles{
		EnglishJSON: englishJSONPath,
		EnglishTXT:  englishTXTPath,
		EnglishSRT:  englishSRTPath,
	}
	if len(reviewTurns) > 0 {
		chineseJSONPath := filepath.Join(outputDir, "chinese_turns.json")
		chineseTXTPath := filepath.Join(outputDir, "chinese_turns.txt")
		reviewJSONPath := filepath.Join(outputDir, "review_turns.json")
		reviewTXTPath := filepath.Join(outputDir, "review_turns.txt")

		originalPayload := make([]importJSONTurn, 0, len(reviewTurns))
		chineseLines := make([]string, 0, len(reviewTurns))
		for _, turn := range reviewTurns {
			originalPayload = append(originalPayload, importJSONTurn{
				TurnIndex:    turn.TurnIndex,
				Speaker:      turn.Speaker,
				Start:        turn.Start,
				End:          turn.End,
				StartTS:      turn.StartTS,
				EndTS:        turn.EndTS,
				OriginalText: turn.OriginalText,
				ZHText:       turn.OriginalText,
			})
			chineseLines = append(chineseLines, fmt.Sprintf("[%s - %s] %s: %s", turn.StartTS, turn.EndTS, turn.Speaker, turn.OriginalText))
		}

		if err := writeIndentedJSONFile(chineseJSONPath, originalPayload); err != nil {
			return TranslationManifest{}, err
		}
		if err := os.WriteFile(chineseTXTPath, []byte(strings.Join(chineseLines, "\n")+"\n"), 0o644); err != nil {
			return TranslationManifest{}, err
		}
		if err := saveReviewTurns(reviewJSONPath, reviewTurns); err != nil {
			return TranslationManifest{}, err
		}
		if err := os.WriteFile(reviewTXTPath, []byte(renderReviewText(reviewTurns)), 0o644); err != nil {
			return TranslationManifest{}, err
		}

		files.ChineseJSON = chineseJSONPath
		files.ChineseTXT = chineseTXTPath
		files.ReviewJSON = reviewJSONPath
		files.ReviewTXT = reviewTXTPath
	}

	turnCount := len(reviewTurns)
	if turnCount == 0 {
		turnCount = countDistinctSourceTurns(segments)
	}
	manifest := translationManifestFile{
		InputAudio:  inputAudio,
		OutputDir:   outputDir,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Turns:       turnCount,
		Segments:    len(segments),
		Files:       files,
	}
	if err := writeIndentedJSONFile(manifestPath, manifest); err != nil {
		return TranslationManifest{}, err
	}
	return loadTranslationManifest(manifestPath)
}

func renderReviewText(turns []ReviewTurn) string {
	blocks := make([]string, 0, len(turns))
	for _, turn := range turns {
		lines := []string{
			fmt.Sprintf("[%s - %s] %s", turn.StartTS, turn.EndTS, turn.Speaker),
			"原始转写: " + turn.OriginalText,
			"AI 校对: " + turn.ReviewedText,
		}
		for _, issue := range turn.Issues {
			prefix := strings.TrimSpace(strings.Trim(strings.Join([]string{issue.Category, issue.Severity}, "/"), "/"))
			if prefix == "" {
				prefix = "提示"
			}
			reason := strings.TrimSpace(issue.Reason)
			if reason == "" {
				reason = strings.TrimSpace(issue.Suggestion)
			}
			lines = append(lines, fmt.Sprintf("- [%s] %s", prefix, reason))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func renderSRT(segments []importedEnglishSegment) string {
	blocks := make([]string, 0, len(segments))
	for index, segment := range segments {
		speaker := strings.TrimSpace(strings.TrimPrefix(segment.Speaker, "Speaker "))
		text := strings.TrimSpace(segment.EnText)
		if speaker != "" {
			text = speaker + ": " + text
		}
		blocks = append(blocks, strings.Join([]string{
			strconv.Itoa(index + 1),
			toSRTTimestamp(segment.Start) + " --> " + toSRTTimestamp(segment.End),
			text,
		}, "\n"))
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func writeIndentedJSONFile(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func splitTextBlocks(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	parts := strings.Split(normalized, "\n\n")
	blocks := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			blocks = append(blocks, part)
		}
	}
	return blocks
}

func splitNonEmptyLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = cleanImportText(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseTimedSpeakerHeader(line string) (string, string, string, bool) {
	match := importTimedLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 4 {
		return "", "", "", false
	}
	speaker, ok := parseSpeakerLabel(match[3])
	if !ok {
		return "", "", "", false
	}
	return match[1], match[2], speaker, true
}

func parseTimedSpeakerLine(line string) (string, string, string, string, bool) {
	match := importTimedLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 4 {
		return "", "", "", "", false
	}

	payload := cleanImportText(match[3])
	if nested := importTimedLinePattern.FindStringSubmatch(payload); len(nested) == 4 {
		if speaker, text, ok := parseSpeakerFromPayload(nested[3]); ok {
			return nested[1], nested[2], speaker, text, true
		}
	}

	speaker, text, ok := parseSpeakerFromPayload(payload)
	if !ok {
		return "", "", "", "", false
	}
	if nested := importTimedLinePattern.FindStringSubmatch(text); len(nested) == 4 {
		if nestedSpeaker, nestedText, nestedOK := parseSpeakerFromPayload(nested[3]); nestedOK {
			return nested[1], nested[2], nestedSpeaker, nestedText, true
		}
		return nested[1], nested[2], speaker, cleanImportText(nested[3]), true
	}
	return match[1], match[2], speaker, text, true
}

func parseSpeakerTextLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if match := importExplicitSpeakerPattern.FindStringSubmatch(line); len(match) == 3 {
		text := cleanImportText(match[2])
		if text != "" {
			return "Speaker " + strings.ToUpper(strings.TrimSpace(match[1])), text, true
		}
	}
	if match := importSimpleSpeakerPattern.FindStringSubmatch(line); len(match) == 3 {
		text := cleanImportText(match[2])
		if text != "" {
			return strings.TrimSpace(match[1]), text, true
		}
	}

	match := importSpeakerTextPattern.FindStringSubmatch(line)
	if len(match) != 3 {
		return "", "", false
	}
	text := cleanImportText(match[2])
	return match[1], text, text != ""
}

func parseSpeakerFromPayload(payload string) (string, string, bool) {
	if speaker, text, ok := parseSpeakerTextLine(payload); ok {
		return speaker, text, true
	}
	return "", "", false
}

func parseSpeakerLabel(value string) (string, bool) {
	value = cleanImportText(strings.TrimSuffix(value, ":"))
	if value == "" {
		return "", false
	}
	return value, true
}

func (m *importSpeakerMapper) resolve(raw string) (string, error) {
	if m.mapping == nil {
		m.mapping = map[string]string{}
	}
	key := normalizeSpeakerKey(raw)
	if key == "" {
		key = "__DEFAULT__"
	}
	if mapped, ok := m.mapping[key]; ok {
		return mapped, nil
	}

	if preset := presetSpeakerLabelForKey(key); preset != "" {
		if m.isSpeakerTaken(preset) {
			for existingKey, existingValue := range m.mapping {
				if existingValue == preset {
					m.mapping[key] = existingValue
					return existingValue, nil
				}
				if existingKey == key {
					return existingValue, nil
				}
			}
		}
		m.mapping[key] = preset
		return preset, nil
	}

	for _, candidate := range []string{"Speaker A", "Speaker B"} {
		if !m.isSpeakerTaken(candidate) {
			m.mapping[key] = candidate
			return candidate, nil
		}
	}
	return "", errors.New("当前导入内容检测到超过两位说话人，应用目前只支持 A/B 双人播客流程")
}

func (m *importSpeakerMapper) isSpeakerTaken(label string) bool {
	for _, existing := range m.mapping {
		if existing == label {
			return true
		}
	}
	return false
}

func normalizeSpeakerKey(value string) string {
	value = strings.ToUpper(cleanImportText(value))
	value = strings.TrimPrefix(value, "SPEAKER ")
	value = strings.TrimPrefix(value, "SPEAKER")
	value = strings.TrimPrefix(value, "说话人")
	value = strings.TrimPrefix(value, "角色")
	return strings.TrimSpace(value)
}

func presetSpeakerLabelForKey(key string) string {
	switch key {
	case "", "__DEFAULT__", "A", "甲", "1", "01":
		return "Speaker A"
	case "B", "乙", "2", "02":
		return "Speaker B"
	default:
		return ""
	}
}

func normalizeImportedTiming(start float64, end float64, startTS string, endTS string, cursor float64, fallbackDuration float64) (float64, float64, string, string) {
	if parsedStart, err := parseImportTimestamp(startTS); err == nil {
		start = parsedStart
	}
	if parsedEnd, err := parseImportTimestamp(endTS); err == nil {
		end = parsedEnd
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		if start == 0 && strings.TrimSpace(startTS) == "" {
			start = cursor
		}
		end = start + fallbackDuration
	}
	if start < cursor && strings.TrimSpace(startTS) == "" {
		start = cursor
		end = start + fallbackDuration
	}
	return roundMillis(start), roundMillis(end), formatImportTimestamp(start), formatImportTimestamp(end)
}

func parseImportTimestamp(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty timestamp")
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid timestamp: %s", value)
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	secondsPart := strings.SplitN(parts[2], ".", 2)
	if len(secondsPart) != 2 {
		return 0, fmt.Errorf("invalid timestamp: %s", value)
	}
	seconds, err := strconv.Atoi(secondsPart[0])
	if err != nil {
		return 0, err
	}
	millis, err := strconv.Atoi(secondsPart[1])
	if err != nil {
		return 0, err
	}
	return float64(hours*3600+minutes*60+seconds) + float64(millis)/1000, nil
}

func formatImportTimestamp(value float64) string {
	totalMillis := int(roundMillis(value) * 1000)
	hours := totalMillis / 3_600_000
	totalMillis %= 3_600_000
	minutes := totalMillis / 60_000
	totalMillis %= 60_000
	seconds := totalMillis / 1000
	millis := totalMillis % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func toSRTTimestamp(value float64) string {
	return strings.Replace(formatImportTimestamp(value), ".", ",", 1)
}

func estimateDurationSeconds(text string, english bool) float64 {
	length := len([]rune(cleanImportText(text)))
	if length == 0 {
		return 2
	}
	if english {
		words := len(strings.Fields(text))
		if words == 0 {
			words = maxInt(1, length/5)
		}
		duration := 0.9 + float64(words)*0.45
		return clampFloat(duration, 1.8, 9.5)
	}
	duration := 0.9 + float64(length)*0.22
	return clampFloat(duration, 1.6, 10.5)
}

func roundMillis(value float64) float64 {
	return float64(int(value*1000+0.5)) / 1000
}

func cleanImportText(value string) string {
	value = strings.ReplaceAll(value, "\uFEFF", "")
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.TrimSpace(value)
	value = importWhitespacePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func cleanExistingAudioPath(path string) string {
	path = cleanPath(path)
	if path == "" {
		return ""
	}
	if !importAllowedAudioExts[strings.ToLower(filepath.Ext(path))] {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func countDistinctSourceTurns(segments []importedEnglishSegment) int {
	seen := map[int]bool{}
	for _, segment := range segments {
		seen[segment.SourceTurnIndex] = true
	}
	return len(seen)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
