import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import "./App.css";
import { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";
import {
  CancelCurrentJob,
  ClearReferenceAudio,
  GetState,
  OpenPath,
  ReadTextFile,
  SelectAudio,
  SelectOutputDir,
  SelectReferenceAudio,
  StartProcessing,
  StartSynthesis,
  StartTranslation,
} from "../wailsjs/go/main/App";

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
  message: "请选择中文音频，按顺序生成中文稿、英文稿和英文对话音频。",
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

function App() {
  const [state, setState] = useState<JobState>(initialState);
  const [form, setForm] = useState<SynthesisOptions>(initialOptions);
  const [reviewTurns, setReviewTurns] = useState<ReviewTurn[]>([]);
  const [chinesePreview, setChinesePreview] = useState("");
  const [englishPreview, setEnglishPreview] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    GetState().then((next) => setState(next as JobState));
    EventsOn("job:update", (payload: JobState) => setState(payload));
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

  const progressValue = Math.max(0, Math.min(100, Math.round((state.progress || 0) * 100)));
  const statusLabel = STATUS_LABELS[state.status] || state.status || "待命";
  const canProcess = Boolean(state.audioPath) && !state.running && !busy;
  const canTranslate = reviewTurns.length > 0 && !state.running && !busy;
  const canSynthesize = Boolean(state.files.englishTxt || state.englishTranscriptPath) && !state.running && !busy;
  const referenceClipCount = useMemo(() => {
    return (state.result?.speaker_reference_files?.A?.length || 0) + (state.result?.speaker_reference_files?.B?.length || 0);
  }, [state.result]);

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

  return (
    <div className="app-shell">
      <section className="hero-card">
        <div>
          <div className="badge">完整流程</div>
          <h1>中文语音到英文播客音频</h1>
          <p>流程固定为：中文语音 → 中文稿 → 校对 → 英文稿 → 英文对话音频。现在三个阶段都在同一个应用里。</p>
        </div>
        <div className="hero-side">
          <span className={`status-pill status-${state.status || "idle"}`}>{statusLabel}</span>
          <strong>{state.message}</strong>
          <div className="progress-block">
            <div className="progress-label">{progressValue}%</div>
            <div className="progress-bar"><div className={`progress-fill progress-${state.status || "idle"}`} style={{ width: `${progressValue}%` }} /></div>
          </div>
        </div>
      </section>

      <section className="layout">
        <div className="main-column">
          <Panel eyebrow="步骤 1" title="导入中文音频并生成中文稿">
            <PathRow label="中文音频" path={state.audioPath} actionLabel="选择音频" onAction={() => runAction(async () => { await SelectAudio(); })} />
            <PathRow label="输出目录" path={state.outputDir} actionLabel="选择目录" onAction={() => runAction(async () => { await SelectOutputDir(); })} secondaryLabel="打开目录" onSecondary={() => OpenPath(state.outputDir)} />
            <div className="action-row">
              <button className="button primary" disabled={!canProcess} onClick={() => runAction(async () => { await StartProcessing(); })}>生成中文稿</button>
              <button className="button ghost" disabled={!state.files.reviewManifest} onClick={() => OpenPath(state.files.reviewManifest)}>打开校对清单</button>
            </div>
            <textarea className="preview" readOnly value={chinesePreview} placeholder="这里会显示中文转写结果。" />
          </Panel>

          <Panel eyebrow="步骤 2" title="校对中文稿并生成英文稿">
            <div className="summary-card">
              <div><span className="summary-label">摘要</span><p>{state.review.summary || "还没有生成可校对的中文稿。"}</p></div>
              <div className="summary-metrics">
                <Metric label="轮次" value={String(state.review.turns.length || 0)} />
                <Metric label="问题数" value={String(state.review.issueCount || 0)} />
              </div>
            </div>
            <div className="editor-list">
              {reviewTurns.length ? reviewTurns.map((turn) => (
                <article className="turn-card" key={turn.turnIndex}>
                  <div className="turn-head">
                    <strong>{turn.speaker}</strong>
                    <span>{turn.startTs} - {turn.endTs}</span>
                  </div>
                  <div className="turn-copy"><span>原始稿</span><p>{turn.originalText}</p></div>
                  <label className="field">
                    <span>校对稿</span>
                    <textarea value={turn.reviewedText} onChange={(event) => updateReviewTurn(turn.turnIndex, event.target.value)} />
                  </label>
                  {turn.issues?.length ? (
                    <div className="issue-list">
                      {turn.issues.map((issue, index) => (
                        <div className="issue-chip" key={`${turn.turnIndex}-${index}`}>{issue.category} · {issue.severity} · {issue.reason}</div>
                      ))}
                    </div>
                  ) : null}
                </article>
              )) : <div className="empty-box">先完成步骤 1，校对稿会显示在这里。</div>}
            </div>
            <div className="action-row">
              <button className="button primary" disabled={!canTranslate} onClick={() => runAction(async () => { await StartTranslation(reviewTurns); })}>生成英文稿</button>
              <button className="button ghost" disabled={!state.files.reviewTxt} onClick={() => OpenPath(state.files.reviewTxt)}>打开校对文本</button>
              <button className="button ghost" disabled={!state.files.resultManifest} onClick={() => OpenPath(state.files.resultManifest)}>打开英文稿清单</button>
            </div>
            <textarea className="preview" readOnly value={englishPreview} placeholder="这里会显示生成后的英文稿。" />
          </Panel>

          <Panel eyebrow="步骤 3" title="生成英文对话音频">
            <PathRow
              label="参考音频"
              path={form.referenceAudioPath || "默认使用原中文音频"}
              actionLabel="更换参考音频"
              onAction={() => runAction(async () => { await SelectReferenceAudio(); })}
              secondaryLabel={form.referenceAudioPath ? "清空" : undefined}
              onSecondary={form.referenceAudioPath ? async () => { await ClearReferenceAudio(); } : undefined}
            />
            <div className="field-grid">
              <Field label="输出文件名前缀"><input value={form.outputBaseName} onChange={(event) => updateField("outputBaseName", event.target.value)} /></Field>
              <Field label="语言代码"><input value={form.language} onChange={(event) => updateField("language", event.target.value)} /></Field>
              <Field label="A 角色音色覆盖"><input value={form.femaleSpeaker} onChange={(event) => updateField("femaleSpeaker", event.target.value)} /></Field>
              <Field label="B 角色音色覆盖"><input value={form.maleSpeaker} onChange={(event) => updateField("maleSpeaker", event.target.value)} /></Field>
            </div>
            <div className="field-grid compact">
              <NumberField label="轮次停顿(ms)" value={form.pauseMs} onChange={(value) => updateField("pauseMs", value)} />
              <NumberField label="同人连读停顿(ms)" value={form.intraTurnPauseMs} onChange={(value) => updateField("intraTurnPauseMs", value)} />
              <NumberField label="语速" value={form.speed} step="0.01" onChange={(value) => updateField("speed", value)} />
              <NumberField label="单段最大字符数" value={form.maxCharsPerUtterance} onChange={(value) => updateField("maxCharsPerUtterance", value)} />
              <NumberField label="单段最大句数" value={form.maxSentencesPerUtterance} onChange={(value) => updateField("maxSentencesPerUtterance", value)} />
            </div>
            <div className="toggle-grid">
              <Toggle checked={form.addConversationMarkers} onChange={(value) => updateField("addConversationMarkers", value)} label="加入语气助词和口语衔接" />
              <Toggle checked={form.preserveTiming} onChange={(value) => updateField("preserveTiming", value)} label="尽量保留时间节奏" />
              <Toggle checked={form.coquiTOSAgreed} onChange={(value) => updateField("coquiTOSAgreed", value)} label="我已同意 XTTS 的 CPML 条款" />
            </div>
            <div className="action-row">
              <button className="button primary" disabled={!canSynthesize} onClick={() => runAction(async () => { await StartSynthesis(form); })}>生成英文音频</button>
              <button className="button warning" disabled={!state.running || busy} onClick={() => runAction(async () => { await CancelCurrentJob(); })}>取消任务</button>
              <button className="button ghost" disabled={!state.outputAudioPath} onClick={() => OpenPath(state.outputAudioPath)}>打开音频</button>
            </div>
          </Panel>
        </div>

        <aside className="side-column">
          <Panel eyebrow="结果" title="当前产物">
            <div className="metric-grid">
              <Metric label="中文轮次" value={String(state.review.turns.length || 0)} />
              <Metric label="英文分段" value={String(state.translation.segments || 0)} />
              <Metric label="参考片段" value={String(referenceClipCount)} />
              <Metric label="采样率" value={state.result.sample_rate ? `${state.result.sample_rate} Hz` : "-"} />
            </div>
            <InfoRow label="英文稿" value={state.files.englishTxt || "尚未生成"} />
            <InfoRow label="英文音频" value={state.outputAudioPath || "尚未生成"} />
            <InfoRow label="当前命令" value={state.commandPreview || "任务开始后显示"} />
            <div className="action-row">
              <button className="button ghost" disabled={!state.files.englishTxt} onClick={() => OpenPath(state.files.englishTxt)}>打开英文稿</button>
              <button className="button ghost" disabled={!state.files.englishSrt} onClick={() => OpenPath(state.files.englishSrt)}>打开字幕</button>
              <button className="button ghost" disabled={!state.manifestPath} onClick={() => OpenPath(state.manifestPath)}>打开当前清单</button>
            </div>
          </Panel>

          <Panel eyebrow="日志" title="运行日志">
            <div className="log-console">
              {state.logs?.length ? state.logs.map((line, index) => <div className="log-line" key={`${index}-${line}`}>{line}</div>) : <div className="empty-box small">Python 输出和进度会显示在这里。</div>}
            </div>
          </Panel>

          <Panel eyebrow="环境" title="运行诊断">
            <InfoRow label="统一 Python" value={state.runtime.pythonExe || "未发现"} />
            <InfoRow label="统一 PYTHONPATH" value={state.runtime.pythonPath || "未发现"} />
            <InfoRow label="HF_HOME" value={state.runtime.hfHome || "未发现"} />
            <InfoRow label="xtts_site" value={state.runtime.xttsSite || "未发现"} />
            <InfoRow label="xtts_src" value={state.runtime.xttsSrc || "未发现"} />
            <InfoRow label="extra site-packages" value={state.runtime.extraSitePackages || "未发现"} />
          </Panel>

          {state.error ? <div className="error-banner">{state.error}</div> : null}
        </aside>
      </section>
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

export default App;
