package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) SaveReviewDraft(turns []ReviewTurn) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, fmt.Errorf("当前已有任务正在运行")
	}
	existingTurns := cloneReviewTurns(a.state.Review.Turns)
	outputDir := a.state.OutputDir
	inputAudio := a.state.AudioPath
	referenceAudio := a.state.ReferenceAudioPath
	summary := strings.TrimSpace(a.state.Review.Summary)
	options := a.state.Options
	runtimeInfo := a.state.Runtime
	a.mu.Unlock()

	if len(turns) == 0 {
		turns = existingTurns
	}
	turns = sanitizeEditableReviewTurns(turns)
	if len(turns) == 0 {
		return a.GetState(), fmt.Errorf("还没有可保存的中文校对稿")
	}

	if strings.TrimSpace(outputDir) == "" {
		outputDir = jobOutputDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return a.GetState(), err
	}

	inputAudio = cleanExistingAudioPath(inputAudio)
	if referenceAudio == "" {
		referenceAudio = inputAudio
	}
	if strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("已保存 %d 段中文校对稿。", len(turns))
	}

	manifest, err := writeImportedChineseArtifacts(outputDir, inputAudio, turns, summary)
	if err != nil {
		return a.GetState(), err
	}

	options = normalizeOptions(options)
	options.OutputDir = outputDir
	options.TranscriptPath = ""
	options.ReferenceAudioPath = referenceAudio

	a.mu.Lock()
	a.cancelRequested = false
	a.currentCmd = nil
	a.state.Running = false
	a.state.Stage = "review"
	a.state.Status = "done"
	a.state.Message = "中文校对稿已保存，可继续生成英文稿。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.AudioPath = inputAudio
	a.state.ReferenceAudioPath = referenceAudio
	a.state.OutputDir = outputDir
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.ManifestPath = manifest.Manifest
	a.state.CommandPreview = ""
	a.state.Files = manifest.Files
	a.state.ReviewManifest = manifest
	a.state.Review = ReviewDraft{
		Summary:    manifest.Summary,
		IssueCount: countIssues(turns),
		Turns:      cloneReviewTurns(turns),
	}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Options = options
	a.state.Runtime = runtimeInfo
	a.appendLogLocked("已保存中文校对稿。")
	a.appendLogLocked("输出目录: " + outputDir)
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()
	return snapshot, nil
}

func (a *App) StartProofread(turns []ReviewTurn) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, fmt.Errorf("当前已有任务正在运行")
	}
	reviewJSON := a.state.Files.ReviewJSON
	outputDir := a.state.OutputDir
	runtimeInfo := a.state.Runtime
	if len(turns) == 0 {
		turns = cloneReviewTurns(a.state.Review.Turns)
	}
	a.mu.Unlock()

	turns = sanitizeEditableReviewTurns(turns)
	if len(turns) == 0 {
		return a.GetState(), fmt.Errorf("还没有可用于 AI 校对的中文稿")
	}

	if strings.TrimSpace(outputDir) == "" {
		outputDir = jobOutputDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return a.GetState(), err
	}
	if strings.TrimSpace(reviewJSON) == "" {
		reviewJSON = filepath.Join(outputDir, "review_turns.json")
	}
	if err := os.MkdirAll(filepath.Dir(reviewJSON), 0o755); err != nil {
		return a.GetState(), err
	}
	if err := saveReviewTurns(reviewJSON, turns); err != nil {
		return a.GetState(), err
	}

	if runtimeInfo.PythonExe == "" {
		var err error
		runtimeInfo, err = resolveRuntimePaths()
		if err != nil {
			return a.GetState(), err
		}
	}
	scriptPath, err := materializeScript("audio_pipeline.py")
	if err != nil {
		return a.GetState(), err
	}
	cmd, preview := buildProofreadCommand(runtimeInfo, scriptPath, reviewJSON, outputDir)
	cmd.Env = buildPythonEnv(runtimeInfo, false)
	cmd.Dir = outputDir

	a.mu.Lock()
	a.cancelRequested = false
	a.state.Running = true
	a.state.Stage = "review"
	a.state.Status = "starting"
	a.state.Message = "正在启动中文稿 AI 校对..."
	a.state.Progress = 0.01
	a.state.Error = ""
	a.state.OutputDir = outputDir
	a.state.ManifestPath = ""
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.CommandPreview = preview
	a.state.Files.ReviewJSON = reviewJSON
	a.state.Options.OutputDir = outputDir
	a.state.Review = ReviewDraft{
		Summary:    a.state.Review.Summary,
		IssueCount: countIssues(turns),
		Turns:      cloneReviewTurns(turns),
	}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Files.EnglishJSON = ""
	a.state.Files.EnglishTXT = ""
	a.state.Files.EnglishSRT = ""
	a.state.Files.ResultManifest = ""
	a.state.Files.OutputAudio = ""
	a.state.Runtime = runtimeInfo
	a.appendLogLocked("中文稿 AI 校对任务已加入队列。")
	a.appendLogLocked("Python: " + runtimeInfo.PythonExe)
	a.appendLogLocked("PYTHONPATH: " + runtimeInfo.PythonPath)
	a.appendLogLocked("执行命令: " + preview)
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()

	go a.runCommand(jobExecution{
		stage:                "review",
		runningMessage:       "正在执行中文稿 AI 校对...",
		command:              cmd,
		expectedManifestPath: filepath.Join(outputDir, "review_manifest.json"),
		manifestPrefixes:     []string{"REVIEW_MANIFEST="},
		onSuccess:            a.completeReview,
	})
	return snapshot, nil
}

func sanitizeEditableReviewTurns(turns []ReviewTurn) []ReviewTurn {
	cleaned := make([]ReviewTurn, 0, len(turns))
	for index, turn := range turns {
		originalText := strings.TrimSpace(turn.OriginalText)
		reviewedText := strings.TrimSpace(turn.ReviewedText)
		if originalText == "" && reviewedText == "" {
			continue
		}
		if originalText == "" {
			originalText = reviewedText
		}
		if reviewedText == "" {
			reviewedText = originalText
		}

		speaker := strings.TrimSpace(turn.Speaker)
		if speaker == "" {
			speaker = "Speaker A"
			if len(cleaned)%2 == 1 {
				speaker = "Speaker B"
			}
		}

		start := turn.Start
		if start < 0 {
			start = 0
		}
		end := turn.End
		if end <= start {
			end = start + estimateDurationSeconds(reviewedText, false)
		}

		startTS := strings.TrimSpace(turn.StartTS)
		if startTS == "" {
			startTS = formatImportTimestamp(start)
		}
		endTS := strings.TrimSpace(turn.EndTS)
		if endTS == "" {
			endTS = formatImportTimestamp(end)
		}

		next := turn
		next.TurnIndex = index
		next.Speaker = speaker
		next.Start = start
		next.End = end
		next.StartTS = startTS
		next.EndTS = endTS
		next.OriginalText = originalText
		next.ReviewedText = reviewedText
		cleaned = append(cleaned, next)
	}

	return cleaned
}
