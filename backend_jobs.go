package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type jobExecution struct {
	stage                string
	runningMessage       string
	command              *exec.Cmd
	expectedManifestPath string
	manifestPrefixes     []string
	onSuccess            func(string) error
}

func (a *App) GetState() JobState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneState(a.state)
}

func (a *App) SelectAudio() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择中文音频",
		Filters: []runtime.FileFilter{{
			DisplayName: "音频文件",
			Pattern:     "*.wav;*.mp3;*.m4a;*.flac;*.aac;*.ogg;*.opus;*.mp4",
		}},
	})
	if err != nil || path == "" {
		return path, err
	}

	outputDir := jobOutputDir()
	options := defaultOptions()
	options.OutputDir = outputDir
	options.ReferenceAudioPath = path

	a.mu.Lock()
	a.state.AudioPath = path
	a.state.ReferenceAudioPath = path
	a.state.OutputDir = outputDir
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.ManifestPath = ""
	a.state.Status = "idle"
	a.state.Stage = "review"
	a.state.Message = "音频已就绪，先生成中文稿并校对。"
	a.state.Progress = 0
	a.state.Error = ""
	a.state.CommandPreview = ""
	a.state.Logs = nil
	a.state.Files = OutputFiles{}
	a.state.Review = ReviewDraft{}
	a.state.ReviewManifest = ReviewManifest{}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Options = options
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()
	return snapshot.AudioPath, nil
}

func (a *App) SelectOutputDir() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择输出文件夹",
	})
	if err != nil || path == "" {
		return path, err
	}

	a.mu.Lock()
	a.state.OutputDir = path
	a.state.Options.OutputDir = path
	a.state.Error = ""
	a.mu.Unlock()
	a.emitState()
	return path, nil
}

func (a *App) SelectReferenceAudio() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择参考音频",
		Filters: []runtime.FileFilter{{
			DisplayName: "音频文件",
			Pattern:     "*.wav;*.mp3;*.m4a;*.flac;*.aac;*.ogg;*.opus;*.mp4",
		}},
	})
	if err != nil || path == "" {
		return path, err
	}

	a.mu.Lock()
	a.state.ReferenceAudioPath = path
	a.state.Options.ReferenceAudioPath = path
	a.state.Error = ""
	a.mu.Unlock()
	a.emitState()
	return path, nil
}

func (a *App) ClearReferenceAudio() JobState {
	a.mu.Lock()
	a.state.ReferenceAudioPath = ""
	a.state.Options.ReferenceAudioPath = ""
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()
	return snapshot
}

func (a *App) StartProcessing() (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前已有任务在运行")
	}
	audioPath := a.state.AudioPath
	outputDir := a.state.OutputDir
	referenceAudio := a.state.ReferenceAudioPath
	a.mu.Unlock()

	audioPath = cleanPath(audioPath)
	if audioPath == "" {
		return a.GetState(), errors.New("请先选择中文音频")
	}
	if _, err := os.Stat(audioPath); err != nil {
		return a.GetState(), fmt.Errorf("中文音频无法读取: %w", err)
	}
	if strings.TrimSpace(outputDir) == "" {
		outputDir = jobOutputDir()
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return a.GetState(), err
	}

	runtimeInfo, err := resolveRuntimePaths()
	if err != nil {
		return a.GetState(), err
	}
	scriptPath, err := materializeScript("audio_pipeline.py")
	if err != nil {
		return a.GetState(), err
	}

	cmd, preview := buildReviewCommand(runtimeInfo, scriptPath, audioPath, outputDir)
	cmd.Env = buildPythonEnv(runtimeInfo, false)
	cmd.Dir = outputDir

	a.mu.Lock()
	a.cancelRequested = false
	a.state.Running = true
	a.state.Stage = "review"
	a.state.Status = "starting"
	a.state.Message = "正在启动中文转写和校对流程..."
	a.state.Progress = 0.01
	a.state.Error = ""
	a.state.AudioPath = audioPath
	a.state.ReferenceAudioPath = referenceAudio
	a.state.OutputDir = outputDir
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.ManifestPath = ""
	a.state.CommandPreview = preview
	a.state.Files = OutputFiles{}
	a.state.Review = ReviewDraft{}
	a.state.ReviewManifest = ReviewManifest{}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Options.OutputDir = outputDir
	a.state.Runtime = runtimeInfo
	a.state.Logs = nil
	a.appendLogLocked("中文稿处理任务已加入队列。")
	a.appendLogLocked("Python: " + runtimeInfo.PythonExe)
	a.appendLogLocked("PYTHONPATH: " + runtimeInfo.PythonPath)
	a.appendLogLocked("执行命令: " + preview)
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()

	go a.runCommand(jobExecution{
		stage:                "review",
		runningMessage:       "正在运行中文转写和校对流程...",
		command:              cmd,
		expectedManifestPath: filepath.Join(outputDir, "review_manifest.json"),
		manifestPrefixes:     []string{"REVIEW_MANIFEST="},
		onSuccess:            a.completeReview,
	})
	return snapshot, nil
}

func (a *App) StartTranslation(turns []ReviewTurn) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前已有任务在运行")
	}
	reviewJSON := a.state.Files.ReviewJSON
	outputDir := a.state.OutputDir
	runtimeInfo := a.state.Runtime
	if len(turns) == 0 {
		turns = cloneReviewTurns(a.state.Review.Turns)
	}
	a.mu.Unlock()

	if len(turns) == 0 {
		return a.GetState(), errors.New("还没有可翻译的校对稿，请先完成中文稿处理")
	}
	if strings.TrimSpace(reviewJSON) == "" {
		if strings.TrimSpace(outputDir) == "" {
			return a.GetState(), errors.New("缺少输出目录，无法启动翻译")
		}
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
	cmd, preview := buildTranslationCommand(runtimeInfo, scriptPath, reviewJSON, outputDir)
	cmd.Env = buildPythonEnv(runtimeInfo, false)
	cmd.Dir = outputDir

	a.mu.Lock()
	a.cancelRequested = false
	a.state.Running = true
	a.state.Stage = "translate"
	a.state.Status = "starting"
	a.state.Message = "正在启动英文稿生成流程..."
	a.state.Progress = 0.01
	a.state.Error = ""
	a.state.ManifestPath = ""
	a.state.EnglishTranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.CommandPreview = preview
	a.state.Review = ReviewDraft{
		Summary:    a.state.Review.Summary,
		IssueCount: countIssues(turns),
		Turns:      cloneReviewTurns(turns),
	}
	a.state.Translation = TranslationManifest{}
	a.state.Result = SynthesisManifest{}
	a.state.Files.OutputAudio = ""
	a.state.Runtime = runtimeInfo
	a.appendLogLocked("英文稿翻译任务已加入队列。")
	a.appendLogLocked("Python: " + runtimeInfo.PythonExe)
	a.appendLogLocked("PYTHONPATH: " + runtimeInfo.PythonPath)
	a.appendLogLocked("执行命令: " + preview)
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()

	go a.runCommand(jobExecution{
		stage:                "translate",
		runningMessage:       "正在生成英文稿...",
		command:              cmd,
		expectedManifestPath: filepath.Join(outputDir, "result_manifest.json"),
		manifestPrefixes:     []string{"RESULT_MANIFEST="},
		onSuccess:            a.completeTranslation,
	})
	return snapshot, nil
}

func (a *App) StartSynthesis(options SynthesisOptions) (JobState, error) {
	a.mu.Lock()
	if a.state.Running {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前已有任务在运行")
	}
	if strings.TrimSpace(options.TranscriptPath) == "" {
		options.TranscriptPath = a.state.EnglishTranscriptPath
		if options.TranscriptPath == "" {
			options.TranscriptPath = a.state.Files.EnglishTXT
		}
	}
	if strings.TrimSpace(options.ReferenceAudioPath) == "" {
		options.ReferenceAudioPath = a.state.ReferenceAudioPath
	}
	if strings.TrimSpace(options.OutputDir) == "" {
		options.OutputDir = a.state.OutputDir
	}
	a.mu.Unlock()

	options = normalizeOptions(options)
	if strings.TrimSpace(options.TranscriptPath) == "" {
		return a.GetState(), errors.New("请先生成英文稿")
	}
	if !options.CoquiTOSAgreed {
		return a.GetState(), errors.New("开始合成前需要先确认 XTTS 的 CPML 条款")
	}
	if _, err := os.Stat(options.TranscriptPath); err != nil {
		return a.GetState(), fmt.Errorf("英文稿无法读取: %w", err)
	}
	if options.ReferenceAudioPath != "" {
		if _, err := os.Stat(options.ReferenceAudioPath); err != nil {
			return a.GetState(), fmt.Errorf("参考音频无法读取: %w", err)
		}
	}

	runtimeInfo, err := resolveRuntimePaths()
	if err != nil {
		return a.GetState(), err
	}
	scriptPath, err := materializeScript("xtts_dialogue_synth.py")
	if err != nil {
		return a.GetState(), err
	}
	outputAudioPath, manifestPath, err := buildOutputPaths(options)
	if err != nil {
		return a.GetState(), err
	}

	cmd, preview := buildSynthesisCommand(runtimeInfo, scriptPath, options, outputAudioPath, manifestPath)
	cmd.Env = buildPythonEnv(runtimeInfo, options.CoquiTOSAgreed)
	cmd.Dir = options.OutputDir

	a.mu.Lock()
	a.cancelRequested = false
	a.state.Running = true
	a.state.Stage = "synthesis"
	a.state.Status = "starting"
	a.state.Message = "正在启动 XTTS 英文对话合成..."
	a.state.Progress = 0.01
	a.state.Error = ""
	a.state.ReferenceAudioPath = options.ReferenceAudioPath
	a.state.OutputDir = options.OutputDir
	a.state.EnglishTranscriptPath = options.TranscriptPath
	a.state.OutputAudioPath = outputAudioPath
	a.state.ManifestPath = manifestPath
	a.state.CommandPreview = preview
	a.state.Options = options
	a.state.Runtime = runtimeInfo
	a.state.Result = SynthesisManifest{}
	a.appendLogLocked("英文对话音频任务已加入队列。")
	a.appendLogLocked("Python: " + runtimeInfo.PythonExe)
	a.appendLogLocked("PYTHONPATH: " + runtimeInfo.PythonPath)
	a.appendLogLocked("执行命令: " + preview)
	snapshot := cloneState(a.state)
	a.mu.Unlock()
	a.emitState()

	go a.runCommand(jobExecution{
		stage:                "synthesis",
		runningMessage:       "正在生成英文对话音频...",
		command:              cmd,
		expectedManifestPath: manifestPath,
		manifestPrefixes:     []string{"RESULT_MANIFEST="},
		onSuccess:            a.completeSynthesis,
	})
	return snapshot, nil
}

func (a *App) CancelCurrentJob() (JobState, error) {
	a.mu.Lock()
	if !a.state.Running || a.currentCmd == nil || a.currentCmd.Process == nil {
		snapshot := cloneState(a.state)
		a.mu.Unlock()
		return snapshot, errors.New("当前没有可取消的任务")
	}
	a.cancelRequested = true
	a.state.Status = "cancelling"
	a.state.Message = "正在停止当前任务..."
	a.appendLogLocked("已请求取消当前任务。")
	snapshot := cloneState(a.state)
	process := a.currentCmd.Process
	a.mu.Unlock()
	a.emitState()

	if err := process.Kill(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (a *App) CancelSynthesis() (JobState, error) {
	return a.CancelCurrentJob()
}

func (a *App) OpenPath(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		a.mu.Lock()
		switch {
		case a.state.OutputAudioPath != "":
			target = a.state.OutputAudioPath
		case a.state.ManifestPath != "":
			target = a.state.ManifestPath
		default:
			target = a.state.OutputDir
		}
		a.mu.Unlock()
	}
	if target == "" {
		return errors.New("当前没有可打开的文件或文件夹")
	}

	info, err := os.Stat(target)
	if err == nil && info.IsDir() {
		return newBackgroundCommand("explorer.exe", target).Start()
	}
	return newBackgroundCommand("explorer.exe", "/select,", target).Start()
}

func (a *App) ReadTextFile(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("路径为空")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) runCommand(job jobExecution) {
	stdout, err := job.command.StdoutPipe()
	if err != nil {
		a.failJob(job.stage, err)
		return
	}
	stderr, err := job.command.StderrPipe()
	if err != nil {
		a.failJob(job.stage, err)
		return
	}

	a.mu.Lock()
	a.currentCmd = job.command
	a.state.Status = "running"
	a.state.Stage = job.stage
	a.state.Message = job.runningMessage
	a.mu.Unlock()
	a.emitState()

	if err := job.command.Start(); err != nil {
		a.failJob(job.stage, err)
		return
	}

	var manifestPath string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		a.scanProcessOutput(stdout, job.manifestPrefixes, &manifestPath)
	}()
	go func() {
		defer wg.Done()
		a.scanProcessOutput(stderr, job.manifestPrefixes, &manifestPath)
	}()
	wg.Wait()
	waitErr := job.command.Wait()

	a.mu.Lock()
	cancelled := a.cancelRequested
	a.cancelRequested = false
	a.currentCmd = nil
	a.mu.Unlock()

	if cancelled {
		a.finishCancelled(job.stage)
		return
	}
	if waitErr != nil {
		a.failJob(job.stage, fmt.Errorf("任务执行失败: %w", waitErr))
		return
	}

	if strings.TrimSpace(manifestPath) == "" {
		if _, err := os.Stat(job.expectedManifestPath); err == nil {
			manifestPath = job.expectedManifestPath
		}
	}
	if strings.TrimSpace(manifestPath) == "" {
		a.failJob(job.stage, errors.New("任务已结束，但没有找到结果清单"))
		return
	}

	if err := job.onSuccess(manifestPath); err != nil {
		a.failJob(job.stage, err)
	}
}

func (a *App) scanProcessOutput(reader io.Reader, manifestPrefixes []string, manifestPath *string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "PROGRESS="):
			a.consumeProgressLine(line)
		default:
			foundManifest := false
			for _, prefix := range manifestPrefixes {
				if strings.HasPrefix(line, prefix) {
					*manifestPath = strings.TrimSpace(strings.TrimPrefix(line, prefix))
					a.updateProgress(0.99, "已检测到结果清单，正在收尾...")
					foundManifest = true
					break
				}
			}
			if !foundManifest {
				a.appendLog(line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		a.appendLog("日志流读取失败: " + err.Error())
	}
}

func (a *App) consumeProgressLine(line string) {
	payload := strings.TrimSpace(strings.TrimPrefix(line, "PROGRESS="))
	valueText, message, found := strings.Cut(payload, "|")
	if !found {
		valueText = payload
		message = ""
	}

	progress, err := strconv.ParseFloat(strings.TrimSpace(valueText), 64)
	if err != nil {
		return
	}
	a.updateProgress(progress, strings.TrimSpace(message))
}

func (a *App) updateProgress(progress float64, message string) {
	a.mu.Lock()
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if progress > a.state.Progress {
		a.state.Progress = progress
	}
	if message != "" {
		a.state.Message = message
		a.appendLogLocked(message)
	}
	a.mu.Unlock()
	a.emitState()
}

func (a *App) completeReview(manifestPath string) error {
	manifest, err := loadReviewManifest(manifestPath)
	if err != nil {
		return err
	}
	turns, err := loadReviewTurns(manifest.Files.ReviewJSON)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.state.Running = false
	a.state.Stage = "review"
	a.state.Status = "done"
	a.state.Message = "中文稿已生成，可继续校对并生成英文稿。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.AudioPath = manifest.InputAudio
	if a.state.ReferenceAudioPath == "" {
		a.state.ReferenceAudioPath = manifest.InputAudio
		a.state.Options.ReferenceAudioPath = manifest.InputAudio
	}
	a.state.OutputDir = manifest.OutputDir
	a.state.ManifestPath = manifestPath
	a.state.Files = manifest.Files
	a.state.ReviewManifest = manifest
	a.state.Review = ReviewDraft{
		Summary:    manifest.Summary,
		IssueCount: countIssues(turns),
		Turns:      turns,
	}
	a.state.Translation = TranslationManifest{}
	a.state.EnglishTranscriptPath = ""
	a.state.Options.TranscriptPath = ""
	a.state.OutputAudioPath = ""
	a.state.Result = SynthesisManifest{}
	a.appendLogLocked("中文稿处理完成。")
	a.mu.Unlock()
	a.emitState()
	return nil
}

func (a *App) completeTranslation(manifestPath string) error {
	manifest, err := loadTranslationManifest(manifestPath)
	if err != nil {
		return err
	}
	turns, err := loadReviewTurns(manifest.Files.ReviewJSON)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.state.Running = false
	a.state.Stage = "translate"
	a.state.Status = "done"
	a.state.Message = "英文稿已生成，可继续生成英文对话音频。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.AudioPath = manifest.InputAudio
	if a.state.ReferenceAudioPath == "" {
		a.state.ReferenceAudioPath = manifest.InputAudio
		a.state.Options.ReferenceAudioPath = manifest.InputAudio
	}
	a.state.OutputDir = manifest.OutputDir
	a.state.ManifestPath = manifestPath
	a.state.Files = manifest.Files
	a.state.Translation = manifest
	a.state.Review.Turns = turns
	a.state.Review.IssueCount = countIssues(turns)
	a.state.EnglishTranscriptPath = manifest.Files.EnglishTXT
	a.state.Options.TranscriptPath = manifest.Files.EnglishTXT
	a.state.Options.OutputDir = manifest.OutputDir
	a.state.OutputAudioPath = ""
	a.state.Result = SynthesisManifest{}
	a.appendLogLocked("英文稿生成完成。")
	a.mu.Unlock()
	a.emitState()
	return nil
}

func (a *App) completeSynthesis(manifestPath string) error {
	manifest, err := loadSynthesisManifest(manifestPath)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.state.Running = false
	a.state.Stage = "synthesis"
	a.state.Status = "done"
	a.state.Message = "英文对话音频已生成完成。"
	a.state.Progress = 1
	a.state.Error = ""
	a.state.ManifestPath = manifestPath
	a.state.OutputAudioPath = manifest.OutputAudio
	a.state.Files.OutputAudio = manifest.OutputAudio
	a.state.Result = manifest
	a.appendLogLocked("英文对话音频生成完成。")
	a.mu.Unlock()
	a.emitState()
	return nil
}

func (a *App) finishCancelled(stage string) {
	a.mu.Lock()
	a.state.Running = false
	a.state.Stage = stage
	a.state.Status = "cancelled"
	a.state.Message = "任务已取消。"
	a.state.Error = ""
	a.appendLogLocked("任务已取消。")
	a.mu.Unlock()
	a.emitState()
}

func (a *App) failJob(stage string, err error) {
	a.mu.Lock()
	a.state.Running = false
	a.state.Stage = stage
	a.state.Status = "error"
	a.state.Error = err.Error()
	a.state.Message = err.Error()
	a.appendLogLocked("错误: " + err.Error())
	a.mu.Unlock()
	a.emitState()
}
