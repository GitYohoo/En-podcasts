package main

import (
	"context"
	"embed"
	"os/exec"
	"sync"
)

const (
	defaultWorkspaceRoot    = ""
	defaultPipelinePython   = ""
	defaultPythonExe        = `D:\models\audio-english-runtime\Scripts\python.exe`
	defaultHFHome           = `D:\models\huggingface`
	defaultXTTSSite         = ""
	defaultXTTSSrc          = ""
	defaultExtraSitePackage = `D:\models\audio-english-runtime\Lib\site-packages`
	defaultJobRoot          = `D:\audio_english_jobs`
	defaultOutputBaseName   = "english_dialogue_xttsv2_podcast_app"
	eventJobUpdate          = "job:update"
	maxLogLines             = 400
)

//go:embed python/audio_pipeline.py python/xtts_dialogue_synth.py
var embeddedScripts embed.FS

type App struct {
	ctx             context.Context
	mu              sync.Mutex
	state           JobState
	currentCmd      *exec.Cmd
	cancelRequested bool
}

func NewApp() *App {
	return &App{
		state: JobState{
			Status:   "idle",
			Stage:    "review",
			Message:  "请选择中文音频，按顺序生成中文稿、英文稿和英文对话音频。",
			Progress: 0,
			Options:  defaultOptions(),
		},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if runtimeInfo, err := resolveRuntimePaths(); err == nil {
		a.mu.Lock()
		a.state.Runtime = runtimeInfo
		a.mu.Unlock()
		a.emitState()
	}
}
