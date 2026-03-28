# 中文语音转英文播客工作台

一个基于 Wails 的桌面应用，用来把中文语音处理成可校对的中文稿、英文稿，以及最终的英文对话音频。

当前流程是完整闭环：

1. 导入中文音频
2. 自动转写中文稿
3. 人工校对中文稿
4. 生成英文稿
5. 生成英文对话音频

应用界面为中文，适合本地离线工作流和批量内容处理。

## 功能介绍

- 中文音频转写
  使用本地 ASR 和说话人分离能力，把输入音频切成带时间戳的多轮对话。

- 中文稿 AI 校对
  对转写结果进行语义级修正，输出更通顺、上下文一致的中文稿，并标注疑点。

- 英文稿生成
  基于校对后的中文稿生成自然口语化英文文本，同时导出字幕文件。

- 英文对话音频合成
  使用 XTTS v2 把英文稿合成为一男一女的英文对话音频，支持播客式表达。

- 参考音频克隆
  默认使用原中文音频作为参考音频，也可以手动指定其他参考音频。

- 节奏和风格控制
  可调停顿、语速、单段字符数、单段句数、口语助词、时间节奏保留等参数。

- 可视化流程管理
  前端内可查看当前阶段、进度、日志、运行环境、输出路径和清单文件。

## 适用场景

- 中文口播转英文播客
- 中文视频对白转英文双人音频
- AI 配音工作流中的中英双语转换
- 本地批量制作对话式英语音频

## 技术栈

- 桌面框架：Wails v2
- 后端：Go 1.23
- 前端：React 18 + TypeScript + Vite
- Python 处理脚本：
  - `python/audio_pipeline.py`
  - `python/xtts_dialogue_synth.py`
- 语音转写：faster-whisper
- 说话人分离：pyannote.audio
- 翻译与文本优化：Transformers 本地模型
- 语音合成：Coqui XTTS v2

## 项目结构

```text
audio-english-desktop/
├─ app.go                        Wails 应用入口状态
├─ backend_jobs.go               任务调度、日志、阶段切换
├─ backend_runtime.go            Python 环境解析、命令构建、路径解析
├─ backend_types.go              前后端共享数据结构
├─ main.go                       桌面程序入口
├─ python/
│  ├─ audio_pipeline.py          中文稿转写、校对、英文稿生成
│  └─ xtts_dialogue_synth.py     英文对话音频合成
├─ frontend/
│  ├─ src/
│  └─ dist/
├─ build/
│  └─ bin/
└─ README.md
```

## 工作流说明

### 第 1 步：导入中文音频

- 支持常见格式：`wav`、`mp3`、`m4a`、`flac`、`aac`、`ogg`、`opus`、`mp4`
- 应用会为当前任务创建一个输出目录
- 默认输出目录根路径是 `D:\Desktop\audio_english_jobs`

任务目录格式示例：

```text
D:\Desktop\audio_english_jobs\job_20260328_120500
```

### 第 2 步：生成和校对中文稿

- 自动执行音频解析
- 自动执行说话人分离
- 自动执行中文转写
- 自动执行中文稿 AI 校对
- 前端可直接编辑校对稿后再继续翻译

### 第 3 步：生成英文稿

- 以校对后的中文稿为输入
- 生成英文文本和字幕
- 输出结果可直接用于音频合成

### 第 4 步：生成英文对话音频

- 默认风格为 `casual-podcast`
- 默认角色设定为 A 女声、B 男声
- 默认参考音频为原中文音频
- 支持替换参考音频、调整语速、调停顿、控制句长

## 输出文件说明

每个任务目录中，常见输出包括：

- `input_audio.*`
  当前任务使用的音频副本

- `chinese_turns.json`
  原始中文轮次 JSON

- `chinese_turns.txt`
  原始中文轮次文本

- `review_turns.json`
  校对后的中文稿 JSON

- `review_turns.txt`
  校对后的中文稿文本

- `review_manifest.json`
  中文稿阶段结果清单

- `english_transcript.json`
  英文稿 JSON

- `english_transcript.txt`
  英文稿文本

- `english_transcript.srt`
  英文字幕文件

- `result_manifest.json`
  英文稿阶段结果清单

- `english_dialogue_xttsv2_podcast_app.wav`
  最终英文对话音频

- `english_dialogue_xttsv2_podcast_app.json`
  音频合成清单

- `_xtts_reference_clips/`
  XTTS 自动截取的参考片段

## 默认运行环境

当前项目按 Windows 本地环境设计，默认会优先使用以下资源：

- Python：
  `D:\models\python310\python.exe`

- Hugging Face 缓存目录：
  `D:\models\huggingface`

- XTTS 站点覆盖目录：
  `C:\Users\yohoo\Desktop\代码\xtts_site`

- XTTS 源码目录：
  `C:\Users\yohoo\Desktop\代码\xtts_src\TTS-0.22.0`

- 额外 Python 包目录：
  `C:\Users\yohoo\Desktop\代码\sherpa-onnx-streaming-zipformer-zh-xlarge-int8-2025-06-30\.venv\Lib\site-packages`

应用现在采用统一 Python 运行时，依赖通过 `PYTHONPATH` 注入，而不是按阶段切换不同 Python。

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

本项目不是开箱即用模板，依赖本地模型和本地 Python 环境。

运行前需要满足：

- Windows 环境
- 已安装 Go 1.23 或兼容版本
- 已安装 Node.js
- 已安装 Wails CLI
- 已准备好统一 Python 环境
- 已准备好相关本地模型或缓存
- `xtts_site`、`xtts_src`、额外 `site-packages` 路径可用

如果 Python 中缺少 `numpy`、`soundfile`、`TTS`、`faster_whisper`、`pyannote.audio` 等依赖，流程会直接失败。

## 开发运行

### 1. 安装前端依赖

在项目根目录执行：

```powershell
cd frontend
npm install
```

### 2. 启动 Wails 开发模式

在项目根目录执行：

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

如果需要显式指定缓存目录，也可以这样构建：

```powershell
$env:GOCACHE='C:\Users\yohoo\Desktop\代码\audio-english-desktop\build\gocache'
wails build -skipbindings -s
```

构建完成后的可执行文件通常位于：

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

- 在校对编辑器中修改 `校对稿`
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

- `D:\models\huggingface` 下是否已有模型缓存
- 路径是否通过环境变量被改错
- 当前 Python 是否和模型缓存路径匹配

### 4. 打开的是旧版本程序

如果你明明改了代码但界面或日志没变化：

- 先关闭旧窗口
- 重新运行 `wails build -skipbindings -s`
- 从 `build\bin\audio-english-desktop.exe` 启动

## 已验证的构建命令

当前项目已经验证过以下命令可通过：

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

- 大模型、缓存和 Python 运行时建议放在 D 盘
- 临时任务输出建议放在 `D:\Desktop\audio_english_jobs`
- 当前代码主要针对 Windows 环境验证
- 本项目依赖较重，不建议随意更换 Python 版本和目录结构

## 后续可扩展方向

- 增加任务历史和恢复能力
- 增加批量任务队列
- 支持更多翻译模型切换
- 支持更多 XTTS 预设风格
- 增加音频试听和波形预览

