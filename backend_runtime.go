package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type pipelineFiles struct {
	EnglishJSON string `json:"english_json"`
	EnglishTXT  string `json:"english_txt"`
	EnglishSRT  string `json:"english_srt"`
	ChineseJSON string `json:"chinese_json"`
	ChineseTXT  string `json:"chinese_txt"`
	ReviewJSON  string `json:"review_json"`
	ReviewTXT   string `json:"review_txt"`
}

type reviewManifestFile struct {
	InputAudio  string        `json:"input_audio"`
	OutputDir   string        `json:"output_dir"`
	GeneratedAt string        `json:"generated_at"`
	Turns       int           `json:"turns"`
	Issues      int           `json:"issues"`
	Summary     string        `json:"summary"`
	Files       pipelineFiles `json:"files"`
}

type translationManifestFile struct {
	InputAudio  string        `json:"input_audio"`
	OutputDir   string        `json:"output_dir"`
	GeneratedAt string        `json:"generated_at"`
	Turns       int           `json:"turns"`
	Segments    int           `json:"segments"`
	Files       pipelineFiles `json:"files"`
}

type reviewIssueFile struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	SourceText string `json:"source_text"`
	Suggestion string `json:"suggestion"`
	Reason     string `json:"reason"`
}

type reviewTurnFile struct {
	TurnIndex    int               `json:"turn_index"`
	Speaker      string            `json:"speaker"`
	Start        float64           `json:"start"`
	End          float64           `json:"end"`
	StartTS      string            `json:"start_ts"`
	EndTS        string            `json:"end_ts"`
	OriginalText string            `json:"original_text"`
	ReviewedText string            `json:"reviewed_text"`
	Issues       []reviewIssueFile `json:"issues"`
}

func (a *App) emitState() {
	a.mu.Lock()
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventJobUpdate, snapshot)
}

func (a *App) appendLog(line string) {
	a.mu.Lock()
	a.appendLogLocked(line)
	a.mu.Unlock()
	a.emitState()
}

func (a *App) appendLogLocked(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	stamped := time.Now().Format("15:04:05") + "  " + line
	a.state.Logs = append(a.state.Logs, stamped)
	if len(a.state.Logs) > maxLogLines {
		a.state.Logs = append([]string(nil), a.state.Logs[len(a.state.Logs)-maxLogLines:]...)
	}
}

func jobOutputDir() string {
	root := defaultJobRoot
	if envValue := strings.TrimSpace(os.Getenv("AUDIO_ENGLISH_JOB_ROOT")); envValue != "" {
		root = envValue
	}
	return filepath.Join(root, "job_"+time.Now().Format("20060102_150405"))
}

func buildReviewCommand(runtimeInfo RuntimeInfo, scriptPath string, audioPath string, outputDir string) (*exec.Cmd, string) {
	args := []string{
		scriptPath,
		"--mode", "review",
		"--input", audioPath,
		"--output-dir", outputDir,
		"--hf-home", runtimeInfo.HFHome,
		"--num-speakers", "2",
	}
	commandPreview := quoteWindowsCommand(append([]string{runtimeInfo.PythonExe}, args...))
	return newBackgroundCommand(runtimeInfo.PythonExe, args...), commandPreview
}

func buildTranslationCommand(runtimeInfo RuntimeInfo, scriptPath string, reviewJSON string, outputDir string) (*exec.Cmd, string) {
	args := []string{
		scriptPath,
		"--mode", "translate",
		"--review-json", reviewJSON,
		"--output-dir", outputDir,
		"--hf-home", runtimeInfo.HFHome,
	}
	commandPreview := quoteWindowsCommand(append([]string{runtimeInfo.PythonExe}, args...))
	return newBackgroundCommand(runtimeInfo.PythonExe, args...), commandPreview
}

func buildSynthesisCommand(runtimeInfo RuntimeInfo, scriptPath string, options SynthesisOptions, outputAudioPath string, manifestPath string) (*exec.Cmd, string) {
	args := []string{
		scriptPath,
		"--transcript", options.TranscriptPath,
		"--output", outputAudioPath,
		"--manifest", manifestPath,
		"--hf-home", runtimeInfo.HFHome,
		"--xtts-site", runtimeInfo.XTTSSite,
		"--xtts-src", runtimeInfo.XTTSSrc,
		"--extra-site-packages", runtimeInfo.ExtraSitePackages,
		"--style", options.Style,
		"--language", options.Language,
		"--pause-ms", strconv.Itoa(options.PauseMS),
		"--intra-turn-pause-ms", strconv.Itoa(options.IntraTurnPauseMS),
		"--speed", strconv.FormatFloat(options.Speed, 'f', -1, 64),
		"--max-chars-per-utterance", strconv.Itoa(options.MaxCharsPerUtterance),
		"--max-sentences-per-utterance", strconv.Itoa(options.MaxSentencesPerUtterance),
	}
	if options.ReferenceAudioPath != "" {
		args = append(args, "--reference-audio", options.ReferenceAudioPath)
	}
	if options.PreserveTiming {
		args = append(args, "--preserve-timing")
	}
	if options.AddConversationMarkers {
		args = append(args, "--add-conversation-markers")
	}
	if options.CoquiTOSAgreed {
		args = append(args, "--coqui-tos-agreed")
	}
	if options.FemaleSpeaker != "" {
		args = append(args, "--female-speaker", options.FemaleSpeaker)
	}
	if options.MaleSpeaker != "" {
		args = append(args, "--male-speaker", options.MaleSpeaker)
	}

	commandPreview := quoteWindowsCommand(append([]string{runtimeInfo.PythonExe}, args...))
	return newBackgroundCommand(runtimeInfo.PythonExe, args...), commandPreview
}

func buildPythonEnv(runtimeInfo RuntimeInfo, tosAgreed bool) []string {
	pythonPath := composePythonPath(runtimeInfo)
	env := append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
		"HF_HOME="+runtimeInfo.HFHome,
		"HUGGINGFACE_HUB_CACHE="+filepath.Join(runtimeInfo.HFHome, "hub"),
		"HF_HUB_OFFLINE=1",
		"TRANSFORMERS_OFFLINE=1",
	)
	if pythonPath != "" {
		env = append(env, "PYTHONPATH="+pythonPath)
	}
	if runtimeInfo.XTTSSite != "" {
		env = append(env, "XTTS_APP_SITE="+runtimeInfo.XTTSSite)
	}
	if runtimeInfo.XTTSSrc != "" {
		env = append(env, "XTTS_APP_SRC="+runtimeInfo.XTTSSrc)
	}
	if runtimeInfo.ExtraSitePackages != "" {
		env = append(env, "XTTS_APP_EXTRA_SITE="+runtimeInfo.ExtraSitePackages)
	}
	if tosAgreed {
		env = append(env, "COQUI_TOS_AGREED=1")
	}
	return env
}

func composePythonPath(runtimeInfo RuntimeInfo) string {
	entries := []string{
		runtimeInfo.XTTSSite,
		runtimeInfo.XTTSSrc,
		runtimeInfo.ExtraSitePackages,
	}
	if existing := strings.TrimSpace(os.Getenv("PYTHONPATH")); existing != "" {
		for _, item := range strings.Split(existing, string(os.PathListSeparator)) {
			entries = append(entries, item)
		}
	}

	seen := map[string]bool{}
	ordered := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		entry = filepath.Clean(entry)
		if seen[entry] {
			continue
		}
		seen[entry] = true
		ordered = append(ordered, entry)
	}
	return strings.Join(ordered, string(os.PathListSeparator))
}

func buildOutputPaths(options SynthesisOptions) (string, string, error) {
	if strings.TrimSpace(options.OutputDir) == "" {
		return "", "", errors.New("输出目录为空")
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return "", "", err
	}
	outputAudioPath := filepath.Join(options.OutputDir, options.OutputBaseName+".wav")
	manifestPath := filepath.Join(options.OutputDir, options.OutputBaseName+".json")
	return outputAudioPath, manifestPath, nil
}

func manifestFiles(files pipelineFiles, reviewManifest string, resultManifest string, outputAudio string) OutputFiles {
	return OutputFiles{
		EnglishJSON:    files.EnglishJSON,
		EnglishTXT:     files.EnglishTXT,
		EnglishSRT:     files.EnglishSRT,
		ChineseJSON:    files.ChineseJSON,
		ChineseTXT:     files.ChineseTXT,
		ReviewJSON:     files.ReviewJSON,
		ReviewTXT:      files.ReviewTXT,
		ReviewManifest: reviewManifest,
		ResultManifest: resultManifest,
		OutputAudio:    outputAudio,
	}
}

func loadReviewManifest(path string) (ReviewManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReviewManifest{}, err
	}
	var manifest reviewManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ReviewManifest{}, err
	}
	return ReviewManifest{
		InputAudio:  manifest.InputAudio,
		OutputDir:   manifest.OutputDir,
		GeneratedAt: manifest.GeneratedAt,
		Turns:       manifest.Turns,
		Issues:      manifest.Issues,
		Summary:     manifest.Summary,
		Manifest:    path,
		Files:       manifestFiles(manifest.Files, path, "", ""),
	}, nil
}

func loadTranslationManifest(path string) (TranslationManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TranslationManifest{}, err
	}
	var manifest translationManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return TranslationManifest{}, err
	}
	return TranslationManifest{
		InputAudio:  manifest.InputAudio,
		OutputDir:   manifest.OutputDir,
		GeneratedAt: manifest.GeneratedAt,
		Turns:       manifest.Turns,
		Segments:    manifest.Segments,
		Manifest:    path,
		Files:       manifestFiles(manifest.Files, "", path, ""),
	}, nil
}

func loadSynthesisManifest(path string) (SynthesisManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SynthesisManifest{}, err
	}
	var manifest SynthesisManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return SynthesisManifest{}, err
	}
	manifest.ManifestPath = path
	return manifest, nil
}

func loadReviewTurns(path string) ([]ReviewTurn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []reviewTurnFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	turns := make([]ReviewTurn, 0, len(raw))
	for _, item := range raw {
		issues := make([]ReviewIssue, 0, len(item.Issues))
		for _, issue := range item.Issues {
			issues = append(issues, ReviewIssue{
				Category:   issue.Category,
				Severity:   issue.Severity,
				SourceText: issue.SourceText,
				Suggestion: issue.Suggestion,
				Reason:     issue.Reason,
			})
		}
		turns = append(turns, ReviewTurn{
			TurnIndex:    item.TurnIndex,
			Speaker:      item.Speaker,
			Start:        item.Start,
			End:          item.End,
			StartTS:      item.StartTS,
			EndTS:        item.EndTS,
			OriginalText: item.OriginalText,
			ReviewedText: item.ReviewedText,
			Issues:       issues,
		})
	}
	return turns, nil
}

func saveReviewTurns(path string, turns []ReviewTurn) error {
	payload := make([]reviewTurnFile, 0, len(turns))
	for _, turn := range turns {
		issues := make([]reviewIssueFile, 0, len(turn.Issues))
		for _, issue := range turn.Issues {
			issues = append(issues, reviewIssueFile{
				Category:   strings.TrimSpace(issue.Category),
				Severity:   strings.TrimSpace(issue.Severity),
				SourceText: strings.TrimSpace(issue.SourceText),
				Suggestion: strings.TrimSpace(issue.Suggestion),
				Reason:     strings.TrimSpace(issue.Reason),
			})
		}
		payload = append(payload, reviewTurnFile{
			TurnIndex:    turn.TurnIndex,
			Speaker:      strings.TrimSpace(turn.Speaker),
			Start:        turn.Start,
			End:          turn.End,
			StartTS:      strings.TrimSpace(turn.StartTS),
			EndTS:        strings.TrimSpace(turn.EndTS),
			OriginalText: strings.TrimSpace(turn.OriginalText),
			ReviewedText: strings.TrimSpace(turn.ReviewedText),
			Issues:       issues,
		})
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func countIssues(turns []ReviewTurn) int {
	total := 0
	for _, turn := range turns {
		total += len(turn.Issues)
	}
	return total
}

func materializeScript(name string) (string, error) {
	data, err := embeddedScripts.ReadFile("python/" + name)
	if err != nil {
		return "", err
	}
	runtimeDir := filepath.Join(os.TempDir(), "audio-english-desktop")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(runtimeDir, name)
	if err := os.WriteFile(scriptPath, data, 0o644); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func resolveRuntimePaths() (RuntimeInfo, error) {
	roots := candidateRoots()

	pythonExe, err := resolvePythonPath(roots)
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("未找到统一 Python 运行时: %w", err)
	}

	xttsSite, err := resolveExistingPath("XTTS_APP_SITE", append([]string{
		defaultXTTSSite,
	}, candidatePaths(roots, "xtts_site")...))
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("未找到 xtts_site 目录: %w", err)
	}

	xttsSrc, err := resolveExistingPath("XTTS_APP_SRC", append([]string{
		defaultXTTSSrc,
	}, candidatePaths(roots, "xtts_src", "TTS-0.22.0")...))
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("未找到修补后的 Coqui 源码目录: %w", err)
	}

	extraSite, err := resolveExistingPath("XTTS_APP_EXTRA_SITE", append([]string{
		defaultExtraSitePackage,
	}, candidatePaths(roots, "sherpa-onnx-streaming-zipformer-zh-xlarge-int8-2025-06-30", ".venv", "Lib", "site-packages")...))
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("未找到额外的 site-packages 目录: %w", err)
	}

	hfHome := strings.TrimSpace(os.Getenv("XTTS_APP_HF_HOME"))
	if hfHome == "" {
		hfHome = defaultHFHome
	}
	if err := os.MkdirAll(hfHome, 0o755); err != nil {
		return RuntimeInfo{}, fmt.Errorf("无法准备 HF_HOME 目录: %w", err)
	}

	runtimeInfo := RuntimeInfo{
		PythonExe:         pythonExe,
		HFHome:            filepath.Clean(hfHome),
		XTTSSite:          xttsSite,
		XTTSSrc:           xttsSrc,
		ExtraSitePackages: extraSite,
	}
	runtimeInfo.PythonPath = composePythonPath(runtimeInfo)
	return runtimeInfo, nil
}

func resolvePythonPath(roots []string) (string, error) {
	for _, envKey := range []string{"AUDIO_ENGLISH_APP_PYTHON", "XTTS_APP_PYTHON", "AUDIO_PIPELINE_PYTHON"} {
		if envValue := strings.TrimSpace(os.Getenv(envKey)); envValue != "" {
			if _, err := os.Stat(envValue); err != nil {
				return "", fmt.Errorf("%s 指向 %q，但该路径无法访问: %w", envKey, envValue, err)
			}
			return filepath.Clean(envValue), nil
		}
	}

	candidates := append([]string{
		defaultPythonExe,
		defaultPipelinePython,
		filepath.Join(defaultWorkspaceRoot, "sherpa-onnx-streaming-zipformer-zh-xlarge-int8-2025-06-30", ".venv", "Scripts", "python.exe"),
	}, candidatePaths(roots, "sherpa-onnx-streaming-zipformer-zh-xlarge-int8-2025-06-30", ".venv", "Scripts", "python.exe")...)
	return resolveExistingPath("", candidates)
}

func candidateRoots() []string {
	candidates := []string{defaultWorkspaceRoot}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exePath))
	}

	roots := []string{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		current := filepath.Clean(candidate)
		for i := 0; i < 6; i++ {
			if !seen[current] {
				seen[current] = true
				roots = append(roots, current)
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return roots
}

func candidatePaths(roots []string, parts ...string) []string {
	candidates := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		candidates = append(candidates, filepath.Join(append([]string{root}, parts...)...))
		candidates = append(candidates, filepath.Join(append([]string{root, ".."}, parts...)...))
	}
	return candidates
}

func resolveExistingPath(envKey string, candidates []string) (string, error) {
	if envKey != "" {
		if envValue := strings.TrimSpace(os.Getenv(envKey)); envValue != "" {
			if _, err := os.Stat(envValue); err != nil {
				return "", fmt.Errorf("%s 指向 %q，但该路径无法访问: %w", envKey, envValue, err)
			}
			return filepath.Clean(envValue), nil
		}
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("没有找到可用路径")
}

func quoteWindowsCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.ReplaceAll(arg, `"`, `\"`)
		if strings.ContainsAny(arg, " \t") {
			arg = `"` + arg + `"`
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}
