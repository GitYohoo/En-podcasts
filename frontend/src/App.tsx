import type { ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import "./App.css";
import { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";
import { main as WailsModels } from "../wailsjs/go/models";
import {
  CancelCurrentJob,
  ClearReferenceAudio,
  GetState,
  ImportChineseTextFile,
  ImportEnglishTranscriptFile,
  OpenPath,
  ReadTextFile,
  SaveReviewDraft,
  SelectAudio,
  SelectOutputDir,
  SelectReferenceAudio,
  StartProofread,
  StartProcessing,
  StartSynthesis,
  StartTranslation,
} from "../wailsjs/go/main/App";

type PageId = "hub" | "review" | "synthesis" | "runtime";

type OutputFiles = {
  englishJson: string;
  englishTxt: string;
  englishSrt: string;
  chineseJson: string;
  chineseTxt: string;
  reviewJson: string;
  reviewTxt: string;
  reviewManifest: string;
  resultManifest: string;
  outputAudio: string;
};

type ReviewIssue = {
  category: string;
  severity: string;
  sourceText: string;
  suggestion: string;
  reason: string;
};

type ReviewTurn = {
  turnIndex: number;
  speaker: string;
  start: number;
  end: number;
  startTs: string;
  endTs: string;
  originalText: string;
  reviewedText: string;
  issues: ReviewIssue[];
};

type ReviewDraft = {
  summary: string;
  issueCount: number;
  turns: ReviewTurn[];
};

type ReviewManifest = {
  inputAudio: string;
  outputDir: string;
  generatedAt: string;
  turns: number;
  issues: number;
  summary: string;
  manifest: string;
  files: OutputFiles;
};

type TranslationManifest = {
  inputAudio: string;
  outputDir: string;
  generatedAt: string;
  turns: number;
  segments: number;
  manifest: string;
  files: OutputFiles;
};

type SynthesisOptions = {
  transcriptPath: string;
  referenceAudioPath: string;
  outputDir: string;
  outputBaseName: string;
  style: string;
  addConversationMarkers: boolean;
  preserveTiming: boolean;
  coquiTOSAgreed: boolean;
  language: string;
  pauseMs: number;
  intraTurnPauseMs: number;
  speed: number;
  maxCharsPerUtterance: number;
  maxSentencesPerUtterance: number;
  femaleSpeaker: string;
  maleSpeaker: string;
};

type RuntimeInfo = {
  pythonExe: string;
  pythonPath?: string;
  hfHome: string;
  xttsSite: string;
  xttsSrc: string;
  extraSitePackages: string;
};

type ReferenceSegment = {
  start: number;
  end: number;
  duration: number;
  text: string;
};

type SynthesisManifest = {
  input_transcript: string;
  output_audio: string;
  generated_with: string;
  style: string;
  device: string;
  sample_rate: number;
  pause_ms: number;
  intra_turn_pause_ms: number;
  preserve_timing: boolean;
  add_conversation_markers: boolean;
  speed: number;
  speakers: Record<string, string>;
  reference_audio: string;
  speaker_reference_files: Record<string, string[]>;
  speaker_reference_segments: Record<string, ReferenceSegment[]>;
  available_speakers_sample: string[];
  turns: number;
  runtime_paths: {
    xtts_site: string;
    xtts_src: string;
    extra_site_packages: string[];
  };
  manifest_path?: string;
};

type JobState = {
  running: boolean;
  stage: string;
  status: string;
  message: string;
  progress: number;
  error: string;
  audioPath: string;
  referenceAudioPath: string;
  outputDir: string;
  englishTranscriptPath: string;
  outputAudioPath: string;
  manifestPath: string;
  logs: string[];
  commandPreview: string;
  files: OutputFiles;
  review: ReviewDraft;
  reviewManifest: ReviewManifest;
  translation: TranslationManifest;
  options: SynthesisOptions;
  runtime: RuntimeInfo;
  result: SynthesisManifest;
};

const STATUS_LABELS: Record<string, string> = {
  idle: "待命",
  starting: "启动中",
  running: "执行中",
  cancelling: "取消中",
  cancelled: "已取消",
  done: "已完成",
  error: "出错",
};

const PAGE_META: Array<{ id: PageId; label: string; description: string }> = [
  { id: "hub", label: "入口页", description: "选择起点" },
  { id: "review", label: "中文稿", description: "校对与翻译" },
  { id: "synthesis", label: "英文稿", description: "合成音频" },
  { id: "runtime", label: "运行页", description: "日志与产物" },
];

const initialOptions: SynthesisOptions = {
  transcriptPath: "",
  referenceAudioPath: "",
  outputDir: "",
  outputBaseName: "english_dialogue_xttsv2_podcast_app",
  style: "casual-podcast",
  addConversationMarkers: true,
  preserveTiming: true,
  coquiTOSAgreed: true,
  language: "en",
  pauseMs: 430,
  intraTurnPauseMs: 160,
  speed: 0.98,
  maxCharsPerUtterance: 125,
  maxSentencesPerUtterance: 1,
  femaleSpeaker: "",
  maleSpeaker: "",
};

const emptyFiles: OutputFiles = {
  englishJson: "",
  englishTxt: "",
  englishSrt: "",
  chineseJson: "",
  chineseTxt: "",
  reviewJson: "",
  reviewTxt: "",
  reviewManifest: "",
  resultManifest: "",
  outputAudio: "",
};

const initialState: JobState = {
  running: false,
  stage: "review",
  status: "idle",
  message: "选择一个起点：中文音频、已有中文稿，或已有英文稿。",
  progress: 0,
  error: "",
  audioPath: "",
  referenceAudioPath: "",
  outputDir: "",
  englishTranscriptPath: "",
  outputAudioPath: "",
  manifestPath: "",
  logs: [],
  commandPreview: "",
  files: emptyFiles,
  review: { summary: "", issueCount: 0, turns: [] },
  reviewManifest: { inputAudio: "", outputDir: "", generatedAt: "", turns: 0, issues: 0, summary: "", manifest: "", files: emptyFiles },
  translation: { inputAudio: "", outputDir: "", generatedAt: "", turns: 0, segments: 0, manifest: "", files: emptyFiles },
  options: initialOptions,
  runtime: { pythonExe: "", pythonPath: "", hfHome: "", xttsSite: "", xttsSrc: "", extraSitePackages: "" },
  result: {
    input_transcript: "",
    output_audio: "",
    generated_with: "",
    style: "",
    device: "",
    sample_rate: 0,
    pause_ms: 0,
    intra_turn_pause_ms: 0,
    preserve_timing: false,
    add_conversation_markers: false,
    speed: 0,
    speakers: {},
    reference_audio: "",
    speaker_reference_files: {},
    speaker_reference_segments: {},
    available_speakers_sample: [],
    turns: 0,
    runtime_paths: { xtts_site: "", xtts_src: "", extra_site_packages: [] },
  },
};

function normalizeState(raw: Partial<JobState> | null | undefined): JobState {
  const reviewTurns: ReviewTurn[] = Array.isArray(raw?.review?.turns) ? (raw?.review?.turns as ReviewTurn[]) : [];
  const logs: string[] = Array.isArray(raw?.logs) ? (raw?.logs as string[]) : [];
  const availableSpeakersSample: string[] = Array.isArray(raw?.result?.available_speakers_sample)
    ? (raw?.result?.available_speakers_sample as string[])
    : [];
  const resultRuntimePaths = raw?.result?.runtime_paths || { xtts_site: "", xtts_src: "", extra_site_packages: [] };

  return {
    ...initialState,
    ...raw,
    logs,
    files: {
      ...emptyFiles,
      ...(raw?.files || {}),
    },
    review: {
      summary: raw?.review?.summary || "",
      issueCount: raw?.review?.issueCount || 0,
      turns: reviewTurns,
    },
    reviewManifest: {
      ...initialState.reviewManifest,
      ...(raw?.reviewManifest || {}),
      files: {
        ...emptyFiles,
        ...(raw?.reviewManifest?.files || {}),
      },
    },
    translation: {
      ...initialState.translation,
      ...(raw?.translation || {}),
      files: {
        ...emptyFiles,
        ...(raw?.translation?.files || {}),
      },
    },
    options: {
      ...initialOptions,
      ...(raw?.options || {}),
    },
    runtime: {
      ...initialState.runtime,
      ...(raw?.runtime || {}),
    },
    result: {
      ...initialState.result,
      ...(raw?.result || {}),
      speakers: raw?.result?.speakers || {},
      speaker_reference_files: raw?.result?.speaker_reference_files || {},
      speaker_reference_segments: raw?.result?.speaker_reference_segments || {},
      available_speakers_sample: availableSpeakersSample,
      runtime_paths: {
        xtts_site: resultRuntimePaths.xtts_site || "",
        xtts_src: resultRuntimePaths.xtts_src || "",
        extra_site_packages: Array.isArray(resultRuntimePaths.extra_site_packages)
          ? resultRuntimePaths.extra_site_packages
          : [],
      },
    },
  };
}

function App() {
  const [state, setState] = useState<JobState>(initialState);
  const [form, setForm] = useState<SynthesisOptions>(initialOptions);
  const [reviewTurns, setReviewTurns] = useState<ReviewTurn[]>([]);
  const [chinesePreview, setChinesePreview] = useState("");
  const [englishPreview, setEnglishPreview] = useState("");
  const [busy, setBusy] = useState(false);
  const [page, setPage] = useState<PageId>("hub");
  const [progressCollapsed, setProgressCollapsed] = useState(false);
  const logConsoleRef = useRef<HTMLDivElement | null>(null);
  const stickLogToBottomRef = useRef(true);
  const previousLogCountRef = useRef(0);

  useEffect(() => {
    GetState().then((next) => setState(normalizeState(next as JobState)));
    EventsOn("job:update", (payload: JobState) => setState(normalizeState(payload)));
    return () => EventsOff("job:update");
  }, []);

  useEffect(() => {
    setForm((current) => ({
      ...current,
      ...state.options,
      transcriptPath: state.englishTranscriptPath || state.options?.transcriptPath || current.transcriptPath,
      referenceAudioPath: state.referenceAudioPath || state.options?.referenceAudioPath || current.referenceAudioPath,
      outputDir: state.outputDir || state.options?.outputDir || current.outputDir,
    }));
  }, [state.options, state.englishTranscriptPath, state.referenceAudioPath, state.outputDir]);

  useEffect(() => {
    setReviewTurns(state.review?.turns || []);
  }, [state.review]);

  useEffect(() => {
    const target = state.files.chineseTxt;
    if (!target) {
      setChinesePreview("");
      return;
    }
    ReadTextFile(target).then(setChinesePreview).catch(() => setChinesePreview(""));
  }, [state.files.chineseTxt]);

  useEffect(() => {
    const target = state.files.englishTxt || state.englishTranscriptPath;
    if (!target) {
      setEnglishPreview("");
      return;
    }
    ReadTextFile(target).then(setEnglishPreview).catch(() => setEnglishPreview(""));
  }, [state.files.englishTxt, state.englishTranscriptPath]);

  useEffect(() => {
    const consoleElement = logConsoleRef.current;
    const logCount = state.logs?.length || 0;
    if (!consoleElement) {
      previousLogCountRef.current = logCount;
      return;
    }
    const logsReset = logCount < previousLogCountRef.current;
    if (stickLogToBottomRef.current || logsReset) {
      consoleElement.scrollTop = consoleElement.scrollHeight;
    }
    previousLogCountRef.current = logCount;
  }, [state.logs]);

  useEffect(() => {
    if (state.running) {
      setPage(state.stage === "review" ? "review" : state.stage === "synthesis" ? "runtime" : "synthesis");
      return;
    }
    if (state.outputAudioPath) {
      setPage("runtime");
    } else if (state.files.englishTxt || state.englishTranscriptPath) {
      setPage("synthesis");
    } else if (state.review?.turns?.length) {
      setPage("review");
    }
  }, [state.running, state.stage, state.outputAudioPath, state.files.englishTxt, state.englishTranscriptPath, state.review?.turns?.length]);

  const progressValue = Math.max(0, Math.min(100, Math.round((state.progress || 0) * 100)));
  const statusLabel = STATUS_LABELS[state.status] || state.status || "待命";
  const hasChineseDraft = reviewTurns.length > 0;
  const hasEnglishDraft = Boolean(state.files.englishTxt || state.englishTranscriptPath);
  const canProcess = Boolean(state.audioPath) && !state.running && !busy;
  const canProofread = hasChineseDraft && !state.running && !busy;
  const canSaveReview = hasChineseDraft && !state.running && !busy;
  const canTranslate = hasChineseDraft && !state.running && !busy;
  const canSynthesize = hasEnglishDraft && !state.running && !busy;
  const referenceClipCount = useMemo(
    () => (state.result?.speaker_reference_files?.A?.length || 0) + (state.result?.speaker_reference_files?.B?.length || 0),
    [state.result],
  );
  const ringRadius = 28;
  const ringCircumference = 2 * Math.PI * ringRadius;
  const ringOffset = ringCircumference * (1 - progressValue / 100);

  function updateField<K extends keyof SynthesisOptions>(key: K, value: SynthesisOptions[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function updateReviewTurn(turnIndex: number, reviewedText: string) {
    setReviewTurns((current) =>
      current.map((turn) => (turn.turnIndex === turnIndex ? { ...turn, reviewedText } : turn)),
    );
  }

  async function runAction(action: () => Promise<void>) {
    setBusy(true);
    try {
      await action();
    } finally {
      setBusy(false);
    }
  }

  function formatError(error: unknown) {
    if (error instanceof Error) {
      return error.message;
    }
    return String(error);
  }

  async function handleOpenPath(target: string) {
    if (!target.trim()) {
      return;
    }
    try {
      await OpenPath(target);
    } catch (error) {
      window.alert(`打开失败：${formatError(error)}`);
    }
  }

  function handleLogScroll() {
    const consoleElement = logConsoleRef.current;
    if (!consoleElement) {
      return;
    }
    const distanceToBottom = consoleElement.scrollHeight - consoleElement.scrollTop - consoleElement.clientHeight;
    stickLogToBottomRef.current = distanceToBottom <= 24;
  }

  function toWailsReviewTurns(turns: ReviewTurn[]): WailsModels.ReviewTurn[] {
    return turns.map((turn) => WailsModels.ReviewTurn.createFrom(turn));
  }

  function toWailsSynthesisOptions(options: SynthesisOptions): WailsModels.SynthesisOptions {
    return WailsModels.SynthesisOptions.createFrom(options);
  }

  async function handleImportChinese() {
    await ImportChineseTextFile();
    setPage("review");
  }

  async function handleImportEnglish() {
    await ImportEnglishTranscriptFile();
    setPage("synthesis");
  }

  async function handleStartProcessing() {
    await StartProcessing();
    setPage("review");
  }

  async function handleStartProofread() {
    await StartProofread(toWailsReviewTurns(reviewTurns));
    setPage("review");
  }

  async function handleSaveReviewDraft() {
    await SaveReviewDraft(toWailsReviewTurns(reviewTurns));
    setPage("review");
  }

  async function handleStartTranslation() {
    await StartTranslation(toWailsReviewTurns(reviewTurns));
    setPage("synthesis");
  }

  async function handleStartSynthesis() {
    await StartSynthesis(toWailsSynthesisOptions(form));
    setPage("runtime");
  }

  return (
    <div className="app-shell">
      <header className="hero-panel">
        <div className="hero-copy">
          <div className="hero-kicker">多起点工作台</div>
          <h1>中文语音 / 中文稿 / 英文稿，都可以从中间接着做</h1>
          <p>
            现在不再强制从第一步开始。你可以从中文音频起步，也可以直接导入已有中文稿继续翻译，或者导入已有英文稿直接生成音频。
          </p>
          <div className="hero-chips">
            <HeroChip label="中文稿" value={hasChineseDraft ? `${reviewTurns.length} 段` : "未导入"} />
            <HeroChip label="英文稿" value={hasEnglishDraft ? "已就绪" : "未导入"} />
            <HeroChip label="输出目录" value={state.outputDir || "未设置"} />
          </div>
        </div>
        <div className="hero-status">
          <span className={`status-pill status-${state.status || "idle"}`}>{statusLabel}</span>
          <div className="hero-status-text">{state.message}</div>
          <div className="progress-block">
            <div className="progress-label">{progressValue}%</div>
            <div className="progress-bar">
              <div className={`progress-fill progress-${state.status || "idle"}`} style={{ width: `${progressValue}%` }} />
            </div>
          </div>
          {state.error ? <div className="error-banner compact">{state.error}</div> : null}
        </div>
      </header>

      <nav className="page-tabs">
        {PAGE_META.map((item) => (
          <button
            key={item.id}
            className={`page-tab ${page === item.id ? "active" : ""}`}
            onClick={() => setPage(item.id)}
            type="button"
          >
            <strong>{item.label}</strong>
            <span>{item.description}</span>
          </button>
        ))}
      </nav>

      <main className="page-stack">
        {page === "hub" ? (
          <div className="page-grid hub-grid">
            <Panel eyebrow="起点 A" title="从中文音频开始">
              <p className="panel-intro">完整流程入口。先转中文稿，再校对，再出英文稿和英文音频。</p>
              <PathRow
                label="中文音频"
                path={state.audioPath}
                actionLabel="选择音频"
                onAction={() => runAction(async () => { await SelectAudio(); })}
              />
              <PathRow
                label="输出目录"
                path={state.outputDir}
                actionLabel="选择目录"
                onAction={() => runAction(async () => { await SelectOutputDir(); })}
                secondaryLabel={state.outputDir ? "打开目录" : undefined}
                onSecondary={state.outputDir ? () => { void handleOpenPath(state.outputDir); } : undefined}
              />
              <div className="action-row">
                <button className="button primary" disabled={!canProcess} onClick={() => runAction(handleStartProcessing)}>转写中文稿</button>
                <button className="button ghost" disabled={!state.audioPath} onClick={() => setPage("review")}>去中文稿页</button>
              </div>
            </Panel>

            <Panel eyebrow="起点 B" title="导入已有中文稿">
              <p className="panel-intro">支持导入 `review_turns.json`、`review_turns.txt`、`chinese_turns.txt` 或普通中文文本。</p>
              <div className="hint-card">
                <strong>导入后会自动做什么</strong>
                <p>系统会生成标准化的 `review_turns.json` 和 `review_manifest.json`，你可以直接继续校对或生成英文稿。</p>
              </div>
              <div className="action-row">
                <button className="button primary" disabled={state.running || busy} onClick={() => runAction(handleImportChinese)}>导入中文稿</button>
                <button className="button ghost" disabled={!hasChineseDraft} onClick={() => setPage("review")}>去中文稿页</button>
                <button className="button ghost" disabled={!state.files.reviewManifest} onClick={() => { void handleOpenPath(state.files.reviewManifest); }}>打开清单</button>
              </div>
            </Panel>

            <Panel eyebrow="起点 C" title="导入已有英文稿">
              <p className="panel-intro">支持导入标准 `english_transcript.txt`、`english_transcript.json`、`result_manifest.json`，或普通英文文本。</p>
              <div className="hint-card">
                <strong>普通文本也可以</strong>
                <p>如果没有时间戳，系统会自动补出近似时间轴，并规整成 XTTS 可直接使用的双人对话格式。</p>
              </div>
              <div className="action-row">
                <button className="button primary" disabled={state.running || busy} onClick={() => runAction(handleImportEnglish)}>导入英文稿</button>
                <button className="button ghost" disabled={!hasEnglishDraft} onClick={() => setPage("synthesis")}>去英文稿页</button>
                <button className="button ghost" disabled={!state.files.englishTxt} onClick={() => { void handleOpenPath(state.files.englishTxt); }}>打开英文稿</button>
              </div>
            </Panel>

            <Panel eyebrow="当前上下文" title="当前项目状态">
              <div className="metric-grid">
                <Metric label="中文轮次" value={String(reviewTurns.length || 0)} />
                <Metric label="英文分段" value={String(state.translation.segments || 0)} />
                <Metric label="参考片段" value={String(referenceClipCount)} />
                <Metric label="音频产物" value={state.outputAudioPath ? "已生成" : "未生成"} />
              </div>
              <InfoRow label="当前命令" value={state.commandPreview || "任务开始后显示"} />
              <InfoRow label="英文稿路径" value={state.files.englishTxt || state.englishTranscriptPath || "未准备"} />
              <InfoRow label="音频路径" value={state.outputAudioPath || "未生成"} />
              <div className="action-row">
                <button className="button ghost" disabled={!state.outputAudioPath} onClick={() => { void handleOpenPath(state.outputAudioPath); }}>打开音频</button>
                <button className="button ghost" disabled={!state.manifestPath} onClick={() => { void handleOpenPath(state.manifestPath); }}>打开当前清单</button>
                <button className="button ghost" disabled={!state.logs.length} onClick={() => setPage("runtime")}>查看日志</button>
              </div>
            </Panel>
          </div>
        ) : null}

        {page === "review" ? (
          <div className="page-grid review-grid">
            <Panel eyebrow="中文稿" title="校对中文稿并继续翻译">
              <div className="summary-card">
                <div>
                  <span className="summary-label">摘要</span>
                  <p>{state.review.summary || "这里会显示当前中文稿的摘要。导入中文稿或完成转写后，这里会自动更新。"}</p>
                </div>
                <div className="summary-metrics">
                  <Metric label="轮次" value={String(reviewTurns.length || 0)} />
                  <Metric label="问题数" value={String(state.review.issueCount || 0)} />
                </div>
              </div>

              <div className="action-row">
                <button className="button primary" disabled={!canProofread} onClick={() => runAction(handleStartProofread)}>执行 AI 校对</button>
                <button className="button ghost" disabled={!canSaveReview} onClick={() => runAction(handleSaveReviewDraft)}>保存校对稿</button>
                <button className="button ghost" disabled={!canTranslate} onClick={() => runAction(handleStartTranslation)}>生成英文稿</button>
                <button className="button ghost" disabled={state.running || busy} onClick={() => runAction(handleImportChinese)}>重新导入中文稿</button>
                <button className="button ghost" disabled={!state.files.reviewTxt} onClick={() => { void handleOpenPath(state.files.reviewTxt); }}>打开校对文本</button>
              </div>

              <textarea className="preview tall" readOnly value={chinesePreview} placeholder="这里会显示中文稿预览。" />
              <div className="hint-row">
                <Hint text="如果导入的是普通中文文本，系统会自动补出 A/B 角色和近似时间戳，方便继续走翻译流程。" />
                <Hint text="执行 AI 校对时，会结合前后相邻段落一起判断语义与衔接，不是孤立逐段硬修。" />
                <Hint text="你也可以直接在下面逐段修文，当前校对文本会作为 AI 校对和下一步翻译输入。" />
              </div>
            </Panel>

            <Panel eyebrow="逐段编辑" title="校对编辑器">
              <div className="action-row">
                <button className="button ghost" disabled={!canSaveReview} onClick={() => runAction(handleSaveReviewDraft)}>保存当前编辑</button>
              </div>
              <div className="editor-list">
                {reviewTurns.length ? reviewTurns.map((turn) => (
                  <article className="turn-card" key={turn.turnIndex}>
                    <div className="turn-head">
                      <strong>{turn.speaker}</strong>
                      <span>{turn.startTs} - {turn.endTs}</span>
                    </div>
                    <div className="turn-copy">
                      <span>原始文本</span>
                      <p>{turn.originalText}</p>
                    </div>
                    <label className="field">
                      <span>校对文本</span>
                      <textarea
                        value={turn.reviewedText}
                        onChange={(event) => updateReviewTurn(turn.turnIndex, event.target.value)}
                        placeholder="未执行 AI 校对。可手动填写，或先点击“执行 AI 校对”。"
                      />
                    </label>
                    {turn.issues?.length ? (
                      <div className="issue-list">
                        {turn.issues.map((issue, index) => (
                          <div className="issue-chip" key={`${turn.turnIndex}-${index}`}>
                            {issue.category} / {issue.severity} / {issue.reason}
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </article>
                )) : (
                  <div className="empty-box">还没有可编辑的中文稿。你可以先从入口页导入中文稿，或从中文音频开始生成。</div>
                )}
              </div>
            </Panel>

            <Panel eyebrow="英文稿预览" title="下一步会产出什么">
              <textarea className="preview medium" readOnly value={englishPreview} placeholder="生成英文稿后，这里会显示结果预览。" />
              <div className="action-row">
                <button className="button ghost" disabled={!state.files.resultManifest} onClick={() => { void handleOpenPath(state.files.resultManifest); }}>打开英文稿清单</button>
                <button className="button ghost" disabled={!hasEnglishDraft} onClick={() => setPage("synthesis")}>去英文稿页</button>
              </div>
            </Panel>
          </div>
        ) : null}

        {page === "synthesis" ? (
          <div className="page-grid synthesis-grid">
            <Panel eyebrow="英文稿" title="英文稿与导入入口">
              <PathRow
                label="英文稿文件"
                path={state.files.englishTxt || state.englishTranscriptPath}
                actionLabel="导入英文稿"
                onAction={() => runAction(handleImportEnglish)}
                secondaryLabel={state.files.englishTxt ? "打开文件" : undefined}
                onSecondary={state.files.englishTxt ? () => { void handleOpenPath(state.files.englishTxt); } : undefined}
              />
              <textarea className="preview tall" readOnly value={englishPreview} placeholder="这里会显示英文稿预览。" />
              <div className="hint-row">
                <Hint text="导入 `result_manifest.json` 时，会尽量把中文校对稿一起恢复回来。" />
                <Hint text="导入普通英文文本时，系统会自动整理成标准 `Speaker A/B + 时间戳` 格式。" />
              </div>
            </Panel>

            <Panel eyebrow="音频合成" title="XTTS v2 合成设置">
              <PathRow
                label="参考音频"
                path={form.referenceAudioPath || "未设置，默认使用内置音色"}
                actionLabel="选择参考音频"
                onAction={() => runAction(async () => { await SelectReferenceAudio(); })}
                secondaryLabel={form.referenceAudioPath ? "清空" : undefined}
                onSecondary={form.referenceAudioPath ? async () => { await ClearReferenceAudio(); } : undefined}
              />

              <div className="field-grid">
                <Field label="输出目录"><input value={form.outputDir} onChange={(event) => updateField("outputDir", event.target.value)} /></Field>
                <Field label="输出文件名前缀"><input value={form.outputBaseName} onChange={(event) => updateField("outputBaseName", event.target.value)} /></Field>
                <Field label="A 角色音色覆盖"><input value={form.femaleSpeaker} onChange={(event) => updateField("femaleSpeaker", event.target.value)} /></Field>
                <Field label="B 角色音色覆盖"><input value={form.maleSpeaker} onChange={(event) => updateField("maleSpeaker", event.target.value)} /></Field>
              </div>

              <div className="field-grid compact">
                <NumberField label="轮次停顿(ms)" value={form.pauseMs} onChange={(value) => updateField("pauseMs", value)} />
                <NumberField label="同人停顿(ms)" value={form.intraTurnPauseMs} onChange={(value) => updateField("intraTurnPauseMs", value)} />
                <NumberField label="语速" value={form.speed} step="0.01" onChange={(value) => updateField("speed", value)} />
                <NumberField label="单段最大字符" value={form.maxCharsPerUtterance} onChange={(value) => updateField("maxCharsPerUtterance", value)} />
                <NumberField label="单段最大句数" value={form.maxSentencesPerUtterance} onChange={(value) => updateField("maxSentencesPerUtterance", value)} />
                <Field label="语言代码"><input value={form.language} onChange={(event) => updateField("language", event.target.value)} /></Field>
              </div>

              <div className="toggle-grid">
                <Toggle checked={form.addConversationMarkers} onChange={(value) => updateField("addConversationMarkers", value)} label="增强口语感和语气词" />
                <Toggle checked={form.preserveTiming} onChange={(value) => updateField("preserveTiming", value)} label="尽量保留节奏和时间感" />
                <Toggle checked={form.coquiTOSAgreed} onChange={(value) => updateField("coquiTOSAgreed", value)} label="我已同意 XTTS 的 CPML 条款" />
              </div>

              <div className="action-row">
                <button className="button primary" disabled={!canSynthesize} onClick={() => runAction(handleStartSynthesis)}>生成英文音频</button>
                <button className="button warning" disabled={!state.running || busy} onClick={() => runAction(async () => { await CancelCurrentJob(); })}>取消任务</button>
                <button className="button ghost" disabled={!state.outputAudioPath} onClick={() => { void handleOpenPath(state.outputAudioPath); }}>打开音频</button>
              </div>
            </Panel>

            <Panel eyebrow="结果概览" title="当前英文稿上下文">
              <div className="metric-grid">
                <Metric label="英文分段" value={String(state.translation.segments || 0)} />
                <Metric label="参考片段" value={String(referenceClipCount)} />
                <Metric label="采样率" value={state.result.sample_rate ? `${state.result.sample_rate} Hz` : "-"} />
                <Metric label="音频状态" value={state.outputAudioPath ? "已生成" : "待生成"} />
              </div>
              <InfoRow label="清单路径" value={state.manifestPath || "未生成"} />
              <InfoRow label="英文稿路径" value={state.files.englishTxt || state.englishTranscriptPath || "未准备"} />
              <InfoRow label="输出音频" value={state.outputAudioPath || "未生成"} />
            </Panel>
          </div>
        ) : null}

        {page === "runtime" ? (
          <div className="page-grid runtime-grid">
            <Panel eyebrow="运行日志" title="实时日志">
              <div className="log-console" ref={logConsoleRef} onScroll={handleLogScroll}>
                {state.logs?.length ? state.logs.map((line, index) => <div className="log-line" key={`${index}-${line}`}>{line}</div>) : <div className="empty-box small">运行中的日志会显示在这里。</div>}
              </div>
            </Panel>

            <Panel eyebrow="当前产物" title="文件与结果">
              <InfoRow label="中文稿" value={state.files.chineseTxt || "未准备"} />
              <InfoRow label="校对稿" value={state.files.reviewTxt || "未准备"} />
              <InfoRow label="英文稿" value={state.files.englishTxt || state.englishTranscriptPath || "未准备"} />
              <InfoRow label="英文字幕" value={state.files.englishSrt || "未准备"} />
              <InfoRow label="英文音频" value={state.outputAudioPath || "未生成"} />
              <div className="action-row">
                <button className="button ghost" disabled={!state.files.reviewManifest} onClick={() => { void handleOpenPath(state.files.reviewManifest); }}>打开中文清单</button>
                <button className="button ghost" disabled={!state.files.resultManifest} onClick={() => { void handleOpenPath(state.files.resultManifest); }}>打开英文清单</button>
                <button className="button ghost" disabled={!state.outputAudioPath} onClick={() => { void handleOpenPath(state.outputAudioPath); }}>打开音频</button>
              </div>
            </Panel>

            <Panel eyebrow="运行环境" title="诊断信息">
              <InfoRow label="统一 Python" value={state.runtime.pythonExe || "未发现"} />
              <InfoRow label="PYTHONPATH" value={state.runtime.pythonPath || "未发现"} />
              <InfoRow label="HF_HOME" value={state.runtime.hfHome || "未发现"} />
              <InfoRow label="xtts_site" value={state.runtime.xttsSite || "未发现"} />
              <InfoRow label="xtts_src" value={state.runtime.xttsSrc || "未发现"} />
              <InfoRow label="extra site-packages" value={state.runtime.extraSitePackages || "未发现"} />
            </Panel>
          </div>
        ) : null}
      </main>

      {progressCollapsed ? (
        <button
          className={`floating-progress floating-progress-collapsed status-${state.status || "idle"}`}
          onClick={() => setProgressCollapsed(false)}
          type="button"
          aria-label="展开进度"
        >
          <svg className="progress-ring" viewBox="0 0 72 72" aria-hidden="true">
            <circle className="progress-ring-track" cx="36" cy="36" r={ringRadius} />
            <circle
              className={`progress-ring-fill progress-ring-${state.status || "idle"}`}
              cx="36"
              cy="36"
              r={ringRadius}
              strokeDasharray={ringCircumference}
              strokeDashoffset={ringOffset}
            />
          </svg>
          <div className="floating-progress-mini-label">{progressValue}%</div>
        </button>
      ) : (
        <div className="floating-progress">
          <div className="floating-progress-head">
            <span className={`status-pill status-${state.status || "idle"}`}>{statusLabel}</span>
            <div className="floating-progress-actions">
              <span className="floating-progress-percent">{progressValue}%</span>
              <button className="floating-progress-toggle" onClick={() => setProgressCollapsed(true)} type="button">
                收起
              </button>
            </div>
          </div>
          <div className="floating-progress-text">{state.message}</div>
          <div className="progress-bar floating-progress-bar">
            <div className={`progress-fill progress-${state.status || "idle"}`} style={{ width: `${progressValue}%` }} />
          </div>
        </div>
      )}
    </div>
  );
}

function Panel({ eyebrow, title, children }: { eyebrow: string; title: string; children: ReactNode }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <span className="eyebrow">{eyebrow}</span>
        <h2>{title}</h2>
      </div>
      {children}
    </section>
  );
}

function PathRow({
  label,
  path,
  actionLabel,
  onAction,
  secondaryLabel,
  onSecondary,
}: {
  label: string;
  path: string;
  actionLabel: string;
  onAction: () => void | Promise<void>;
  secondaryLabel?: string;
  onSecondary?: () => void | Promise<void>;
}) {
  return (
    <div className="path-row">
      <div className="path-copy">
        <span className="summary-label">{label}</span>
        <span className="path-value">{path || "未设置"}</span>
      </div>
      <div className="action-row compact">
        <button className="button ghost" onClick={() => void onAction()}>{actionLabel}</button>
        {secondaryLabel && onSecondary ? <button className="button ghost" onClick={() => void onSecondary()}>{secondaryLabel}</button> : null}
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="field"><span>{label}</span>{children}</label>;
}

function NumberField({ label, value, step, onChange }: { label: string; value: number; step?: string; onChange: (value: number) => void }) {
  return <Field label={label}><input type="number" value={Number.isFinite(value) ? value : 0} step={step} onChange={(event) => onChange(Number(event.target.value))} /></Field>;
}

function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (value: boolean) => void; label: string }) {
  return <label className="toggle"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><span>{label}</span></label>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="metric-card"><strong>{value}</strong><span>{label}</span></div>;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return <div className="info-row"><span className="summary-label">{label}</span><span className="path-value">{value}</span></div>;
}

function Hint({ text }: { text: string }) {
  return <div className="hint-chip">{text}</div>;
}

function HeroChip({ label, value }: { label: string; value: string }) {
  return (
    <div className="hero-chip">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export default App;
