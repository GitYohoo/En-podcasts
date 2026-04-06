# 中文语音转英文播客工作台

一个基于 Wails 的桌面应用，用于把中文语音处理成可校对的中文稿、英文稿，以及最终的英文对话音频。

完整流程：

1. 导入中文音频
2. 自动转写中文稿
3. 人工校对中文稿
4. 生成英文稿
5. 生成英文对话音频

## 功能介绍

- 中文音频转写
  使用本地 ASR 和说话人分离能力，把输入音频切成带时间戳的多轮对话。

- 中文稿 AI 校对
  对转写结果进行语义级修正，输出更通顺、上下文一致的中文稿，并标注疑点。
  当前默认本地校对模型为 `Qwen/Qwen3.5-9B`。

- 英文稿生成
  基于校对后的中文稿生成自然口语化英文文本，并导出字幕文件。

- 英文对话音频合成
  使用 XTTS v2 把英文稿合成为一男一女的英文对话音频，支持播客式表达。

- 参考音频克隆
  默认使用原中文音频作为参考音频，也可以手动指定其他参考音频。

- 节奏和风格控制
  可调停顿、语速、单段字符数、单段句数、口语助词、时间节奏保留等参数。

## 技术栈

- 桌面框架：Wails v2
- 后端：Go
- 前端：React + TypeScript + Vite
- Python 处理脚本：
  - `python/audio_pipeline.py`
  - `python/xtts_dialogue_synth.py`
- 语音转写：faster-whisper
- 说话人分离：pyannote.audio
- 翻译与文本优化：Transformers 本地模型
- 语音合成：Coqui XTTS v2

补充说明：
- AI 校对默认使用 `Qwen/Qwen3.5-9B`
- 运行该模型前，建议准备支持 Qwen3.5 的较新 `transformers` 版本以及对应本地模型缓存

## 项目结构

```text
audio-english-desktop/
├─ app.go
├─ backend_jobs.go
├─ backend_runtime.go
├─ backend_types.go
├─ main.go
├─ python/
│  ├─ audio_pipeline.py
│  └─ xtts_dialogue_synth.py
├─ frontend/
│  ├─ src/
│  └─ dist/
├─ build/
│  └─ bin/
└─ README.md
```

## 工作流说明

### 1. 导入中文音频

- 支持常见格式：`wav`、`mp3`、`m4a`、`flac`、`aac`、`ogg`、`opus`、`mp4`
- 应用会为当前任务创建一个输出目录
- 输出目录默认位于你配置的任务根目录下

任务目录示例：

```text
<任务根目录>\job_20260328_120500
```

### 2. 生成和校对中文稿

- 自动执行音频解析
- 自动执行说话人分离
- 自动执行中文转写
- 自动执行中文稿 AI 校对
- 前端可直接编辑校对稿后再继续翻译

### 3. 生成英文稿

- 以校对后的中文稿为输入
- 生成英文文本和字幕
- 输出结果可直接用于音频合成

### 4. 生成英文对话音频

- 默认风格为 `casual-podcast`
- 默认角色设定为 A 女声、B 男声
- 默认参考音频为原中文音频
- 支持替换参考音频、调整语速、调停顿、控制句长

## 输出文件说明

每个任务目录中常见输出包括：

- `input_audio.*`
- `chinese_turns.json`
- `chinese_turns.txt`
- `review_turns.json`
- `review_turns.txt`
- `review_manifest.json`
- `english_transcript.json`
- `english_transcript.txt`
- `english_transcript.srt`
- `result_manifest.json`
- `<输出名前缀>.wav`
- `<输出名前缀>.json`
- `_xtts_reference_clips/`

## 运行环境

当前项目面向 Windows 本地环境设计，依赖：

- 一个统一的 Python 运行时
- 本地 Hugging Face 缓存目录
- `xtts_site`
- `xtts_src/TTS-0.22.0`
- 额外 `site-packages`

应用使用统一 Python 运行时，依赖通过 `PYTHONPATH` 注入，而不是按阶段切换不同 Python。

默认推荐路径：

- Python 运行时：`D:\models\audio-english-runtime\Scripts\python.exe`
- Hugging Face 缓存：`D:\models\huggingface`
- 额外 `site-packages`：`D:\models\audio-english-runtime\Lib\site-packages`

## 可用环境变量

如果你的本地路径不同，可以通过环境变量覆盖默认配置：

- `AUDIO_ENGLISH_APP_PYTHON`
  统一 Python 解释器路径

- `XTTS_APP_PYTHON`
  兼容旧配置的 Python 路径覆盖

- `AUDIO_PIPELINE_PYTHON`
  兼容旧配置的 Python 路径覆盖

- `XTTS_APP_HF_HOME`
  Hugging Face 缓存根目录

- `XTTS_APP_SITE`
  `xtts_site` 路径

- `XTTS_APP_SRC`
  `xtts_src/TTS-0.22.0` 路径

- `XTTS_APP_EXTRA_SITE`
  额外 `site-packages` 路径

- `AUDIO_ENGLISH_JOB_ROOT`
  任务输出根目录

## 运行前提

运行前需要满足：

- Windows 环境
- 已安装 Go
- 已安装 Node.js
- 已安装 Wails CLI
- 已准备好统一 Python 环境
- 已准备好相关本地模型或缓存
- `xtts_site`、`xtts_src`、额外 `site-packages` 路径可用

如果 Python 中缺少 `numpy`、`soundfile`、`TTS`、`faster_whisper`、`pyannote.audio` 等依赖，流程会直接失败。
如果使用 `Qwen/Qwen3.5-9B` 作为 AI 校对模型，建议同时准备较新的 `transformers`，并优先启用 4bit 量化以适配 12GB 显存。

## 开发运行

### 安装前端依赖

```powershell
cd frontend
npm install
```

### 启动开发模式

```powershell
wails dev
```

说明：

- 前端由 Vite 提供热更新
- Wails 会启动桌面应用外壳
- 开发时可以直接修改 Go 和前端代码

## 生产构建

### 前端构建

```powershell
cd frontend
npm run build
```

### Go 构建

```powershell
go build ./...
```

### Wails 打包

```powershell
wails build -skipbindings -s
```

如果需要显式指定缓存目录：

```powershell
$env:GOCACHE='<项目根目录>\build\gocache'
wails build -skipbindings -s
```

构建后的可执行文件通常位于：

```text
build\bin\audio-english-desktop.exe
```

## 界面使用说明

### 步骤 1：生成中文稿

- 点击“选择音频”
- 点击“生成中文稿”
- 等待任务完成
- 在右侧查看日志
- 在中间区域查看中文稿预览

### 步骤 2：校对并生成英文稿

- 在校对编辑器中修改校对稿
- 点击“生成英文稿”
- 生成后可打开英文稿和字幕文件

### 步骤 3：生成英文音频

- 视需要更换参考音频
- 调整播客风格参数
- 确认 XTTS CPML 条款
- 点击“生成英文音频”

## 合成参数说明

- `style`
  当前支持 `default` 和 `casual-podcast`

- `pauseMs`
  不同说话人轮次之间的停顿

- `intraTurnPauseMs`
  同一说话人连续片段之间的停顿

- `speed`
  合成语速

- `maxCharsPerUtterance`
  单段最大字符数

- `maxSentencesPerUtterance`
  单段最大句数

- `addConversationMarkers`
  是否加入语气助词和更口语化的连接

- `preserveTiming`
  是否尽量保留原文时间节奏

- `femaleSpeaker`
  A 角色音色覆盖

- `maleSpeaker`
  B 角色音色覆盖

## 常见问题

### 1. 生成中文稿直接失败

优先检查：

- 日志里是否出现 `ModuleNotFoundError`
- 统一 Python 是否可用
- `PYTHONPATH` 是否已经注入
- `HF_HOME` 是否存在且可读写

### 2. 能转写，不能合成

优先检查：

- `xtts_site` 是否存在
- `xtts_src` 是否存在
- `XTTS_APP_EXTRA_SITE` 指向的 `site-packages` 是否存在
- `COQUI_TOS_AGREED` 是否已勾选
- 参考音频是否可访问

### 3. 模型找不到

优先检查：

- 本地模型缓存目录是否已有对应模型
- 路径是否通过环境变量被改错
- 当前 Python 是否和模型缓存路径匹配

### 4. 打开的是旧版本程序

如果你改了代码但界面或日志没变化：

- 先关闭旧窗口
- 重新运行 `wails build -skipbindings -s`
- 从 `build\bin\audio-english-desktop.exe` 启动

## 已验证的构建命令

```powershell
cd frontend
npm run build
```

```powershell
go build ./...
```

```powershell
wails build -skipbindings -s
```

## 注意事项

- 大模型、缓存和 Python 运行时建议放在非系统盘或独立目录
- 临时任务输出建议放在单独任务目录下
- 当前代码主要针对 Windows 环境验证
- 本项目依赖较重，不建议随意更换 Python 版本和目录结构
