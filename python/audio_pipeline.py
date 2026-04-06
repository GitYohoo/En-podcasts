import argparse
import gc
import importlib.util
import json
import math
import os
import re
import shutil
import sys
import textwrap
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

import numpy as np
import torch
import torchaudio
from faster_whisper import WhisperModel
from faster_whisper.audio import decode_audio
from pyannote.audio import Pipeline as PyannotePipeline
from transformers import (
    AutoModelForCausalLM,
    AutoModelForSeq2SeqLM,
    AutoTokenizer,
    BitsAndBytesConfig,
)


if not hasattr(torchaudio, "list_audio_backends"):
    torchaudio.list_audio_backends = lambda: []

if not hasattr(torchaudio, "get_audio_backend"):
    torchaudio.get_audio_backend = lambda: None

if not hasattr(torchaudio, "set_audio_backend"):
    torchaudio.set_audio_backend = lambda backend: None


DEFAULT_HF_HOME = Path(r"D:\models\huggingface")
DEFAULT_OUTPUT_ROOT = Path(r"D:\Desktop\audio_english_jobs")
DEFAULT_DRAFT_MODEL = "Helsinki-NLP/opus-mt-zh-en"
DEFAULT_REFINE_MODEL = "Qwen/Qwen2.5-3B-Instruct"
DEFAULT_REVIEW_MODEL = "Qwen/Qwen3.5-9B"
DEFAULT_WHISPER_MODEL = "large-v3"
DEFAULT_PYANNOTE_MODEL = "pyannote/speaker-diarization-community-1"
SAMPLE_RATE = 16000
CJK_RE = re.compile(r"[\u4e00-\u9fff]")
THINK_BLOCK_RE = re.compile(r"<think>.*?</think>", flags=re.IGNORECASE | re.DOTALL)
BLACKLIST_PHRASES = (
    "thanks for watching",
    "thank you for watching",
    "please subscribe",
    "like and subscribe",
)
FRAGMENT_GUARD_CHARS = set("嗯啊哦喔诶欸哎喂哈嘿对好行那你我他她吗吧呢呀啦嘞是有无没了恩")
SYSTEM_PROMPT = """You are a professional subtitle translator.
Rewrite spoken Chinese into natural, vivid, idiomatic English subtitles.

Rules:
- Stay faithful to the original meaning.
- Keep the tone conversational and concise.
- Do not add facts, explanations, or closing lines that are not in the source.
- If the draft is literal or awkward, rewrite it naturally.
- Return English only.
"""
REVIEW_SYSTEM_PROMPT = """你擅长把 ASR 转写修成自然、通顺、符合上下文的中文。
你的任务不是机械挑错字，而是结合全文中心主旨、事件线和邻近语境，对当前分段做语义级校对。

校对原则：
- 优先保证当前分段与全文中心主旨、人物关系、事件线一致。
- 保留原句核心意思、说话人口吻和立场，可以适度重组语序，让句子自然顺口。
- 优先修正 ASR 常见错误：同音字、漏字、断句错误、专有名词误识别、前后照应错误。
- 如果局部明显听错或字词混乱，只要上下文支持，可以改写成更合理的表达。
- 只校对当前分段，不得把前后分段的大段内容直接搬到当前分段里。
- 如果当前分段只有一两个字、语气词、确认词或明显残缺的碎片，只能做最小修正，不要扩写成长句。
- 必须保持说话人立场、人称和施受关系正确，不要把“帮我办”改成“我帮你办”，也不要把客户和客服的话术互换。
- 不要凭空新增原文和上下文都没有的事实、数字、专有名词或结论。
- 如果证据不足，就保留不确定部分，不要硬编。
- reviewed_text 必须是可直接阅读的通顺中文，避免口水词堆叠和生硬重复，不要包含括号说明、角色标签或解释备注。
- issues 只列真正修改或仍存疑的点，说明理由要具体。
- 只返回 JSON，不要输出解释。

输出格式：
{
  "reviewed_text": "结合全文语境校对后的中文",
  "issues": [
    {
      "category": "主旨一致性/错别字/术语统一/语病/逻辑疑点/指代不清/表述不顺",
      "severity": "high/medium/low",
      "source_text": "原文片段",
      "suggestion": "建议改法",
      "reason": "为什么需要改"
    }
  ]
}
"""
REVIEW_CONTEXT_PROMPT = """你是中文口播内容总编。请先通读整篇转写，提炼后续校对要用的“全文理解”。

要求：
- 先判断这段音频的中心主旨，再梳理事件线、人物或对象、关键词和明显不确定点。
- 不要编造事实；不确定就直接写“不确定”。
- 表述要简洁，便于后续逐段校对引用。
- 只返回 JSON，不要输出解释。

输出格式：
{
  "summary": "用一两句话概括全文",
  "main_topic": "中心主旨",
  "event_flow": ["事件1", "事件2", "事件3"],
  "participants": ["人物或对象1", "人物或对象2"],
  "keywords": ["关键词1", "关键词2", "关键词3"],
  "uncertainties": ["暂时无法确定的点"]
}
"""


@dataclass
class Turn:
    speaker: str
    start: float
    end: float
    text: str


@dataclass
class ReviewContext:
    summary: str
    brief: str


def log(message: str) -> None:
    print(message, flush=True)


def emit_progress(progress: float, message: str | None = None) -> None:
    progress = max(0.0, min(1.0, float(progress)))
    if message:
        print(f"PROGRESS={progress:.4f}|{message}", flush=True)
    else:
        print(f"PROGRESS={progress:.4f}", flush=True)


def format_ts(seconds: float, srt: bool = False) -> str:
    seconds = max(0.0, float(seconds))
    total_ms = int(round(seconds * 1000))
    hours = total_ms // 3_600_000
    total_ms %= 3_600_000
    minutes = total_ms // 60_000
    total_ms %= 60_000
    secs = total_ms // 1000
    millis = total_ms % 1000
    sep = "," if srt else "."
    return f"{hours:02d}:{minutes:02d}:{secs:02d}{sep}{millis:03d}"


def save_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def normalize_space(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def normalize_zh_text(text: str) -> str:
    text = text.replace("\n", " ").replace("\r", " ")
    text = re.sub(r"(?<=[A-Za-z0-9])(?=[\u4e00-\u9fff])", " ", text)
    text = re.sub(r"(?<=[\u4e00-\u9fff])(?=[A-Za-z0-9])", " ", text)
    text = normalize_space(text)
    text = re.sub(r"\s+([,.!?;:，。！？；：])", r"\1", text)
    text = re.sub(r"(?<=[\u4e00-\u9fff])\s+(?=[\u4e00-\u9fff])", "", text)
    text = re.sub(r"(?<=[\u4e00-\u9fff])\s+(?=[，。！？；：,.!?;:])", "", text)
    text = re.sub(r"(?<=[，。！？；：,.!?;:])\s+(?=[\u4e00-\u9fff])", "", text)
    return text.strip()


def wrap_subtitle(text: str, width: int = 42) -> str:
    lines = textwrap.wrap(text, width=width, break_long_words=False, break_on_hyphens=False)
    return "\n".join(lines) if lines else text


def contains_cjk(text: str) -> bool:
    return bool(CJK_RE.search(text))


def strip_thinking_content(text: str) -> str:
    cleaned = text.strip()
    cleaned = THINK_BLOCK_RE.sub("", cleaned)
    if "</think>" in cleaned.lower():
        lowered = cleaned.lower()
        closing = lowered.rfind("</think>")
        cleaned = cleaned[closing + len("</think>") :]
    return cleaned.strip()


def cleanup_english(text: str) -> str:
    cleaned = strip_thinking_content(text)
    cleaned = re.sub(r"^```(?:text)?\s*", "", cleaned)
    cleaned = re.sub(r"\s*```$", "", cleaned)
    cleaned = re.sub(r"^(Translation|English|Subtitle)\s*:\s*", "", cleaned, flags=re.IGNORECASE)
    cleaned = cleaned.replace("–", "-")
    cleaned = normalize_space(cleaned)
    return cleaned.strip("\"' ")


def apply_chat_template_with_optional_non_thinking(tokenizer: AutoTokenizer, messages: list[dict[str, str]]) -> str:
    try:
        return tokenizer.apply_chat_template(
            messages,
            tokenize=False,
            add_generation_prompt=True,
            enable_thinking=False,
        )
    except TypeError:
        return tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)


def env_flag(name: str) -> bool | None:
    value = os.environ.get(name, "").strip().lower()
    if value in {"1", "true", "yes", "on"}:
        return True
    if value in {"0", "false", "no", "off"}:
        return False
    return None


def has_module(module_name: str) -> bool:
    return importlib.util.find_spec(module_name) is not None


def should_use_4bit_quantization(model_id: str) -> bool:
    override = env_flag("AUDIO_ENGLISH_LLM_4BIT")
    if override is not None:
        return override and torch.cuda.is_available() and has_module("bitsandbytes")
    return torch.cuda.is_available() and "Qwen3.5-" in model_id and has_module("bitsandbytes")


def build_causal_lm_load_kwargs(model_id: str, device: str) -> dict[str, Any]:
    kwargs: dict[str, Any] = {
        "trust_remote_code": True,
        "local_files_only": True,
        "torch_dtype": "auto",
    }
    if device == "cuda":
        kwargs["device_map"] = "auto"
    if should_use_4bit_quantization(model_id):
        compute_dtype = torch.bfloat16 if torch.cuda.is_bf16_supported() else torch.float16
        kwargs["quantization_config"] = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_quant_type="nf4",
            bnb_4bit_use_double_quant=True,
            bnb_4bit_compute_dtype=compute_dtype,
        )
        kwargs["torch_dtype"] = compute_dtype
    return kwargs


def has_blacklisted_phrase(text: str) -> bool:
    lowered = text.lower()
    return any(phrase in lowered for phrase in BLACKLIST_PHRASES)


def choose_whisper_settings() -> tuple[str, str]:
    if torch.cuda.is_available():
        return "cuda", "float16"
    return "cpu", "int8"


def should_enable_word_timestamps() -> bool:
    value = os.environ.get("AUDIO_PIPELINE_ENABLE_WORD_TIMESTAMPS", "").strip().lower()
    return value in {"1", "true", "yes", "on"}


def resolve_local_model_path(model_id: str, hf_home: Path) -> Path:
    repo_dir_name = "models--" + model_id.replace("/", "--")
    candidates = [
        hf_home / repo_dir_name,
        hf_home / "hub" / repo_dir_name,
    ]

    for repo_root in candidates:
        snapshots_dir = repo_root / "snapshots"
        if not snapshots_dir.exists():
            continue

        ref_main = repo_root / "refs" / "main"
        if ref_main.exists():
            snapshot_name = ref_main.read_text(encoding="utf-8").strip()
            snapshot_path = snapshots_dir / snapshot_name
            if snapshot_path.exists():
                return snapshot_path

        snapshots = [child for child in snapshots_dir.iterdir() if child.is_dir()]
        if snapshots:
            snapshots.sort(key=lambda path: path.stat().st_mtime, reverse=True)
            return snapshots[0]

    raise FileNotFoundError(f"未在 {hf_home} 下找到 {model_id} 的本地模型快照")


def decode_input_audio(input_path: Path) -> np.ndarray:
    audio = decode_audio(str(input_path), sampling_rate=SAMPLE_RATE)
    if audio.ndim != 1:
        audio = np.mean(audio, axis=0)
    return audio.astype("float32")


def stage_input_file(input_path: Path, output_dir: Path) -> Path:
    staged = output_dir / f"input_audio{input_path.suffix.lower() or '.wav'}"
    if staged.resolve() != input_path.resolve():
        shutil.copy2(input_path, staged)
    return staged


def diarize_audio(
    audio: np.ndarray,
    model_id: str,
    hf_home: Path,
    num_speakers: int | None,
) -> list[dict[str, Any]]:
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    waveform = torch.from_numpy(audio).unsqueeze(0)
    model_path = resolve_local_model_path(model_id, hf_home)
    pipeline = PyannotePipeline.from_pretrained(str(model_path))
    pipeline.to(device)
    inputs = {"waveform": waveform, "sample_rate": SAMPLE_RATE}
    if num_speakers and num_speakers > 0:
        diarization = pipeline(inputs, num_speakers=num_speakers)
    else:
        diarization = pipeline(inputs)
    annotation = diarization
    if hasattr(diarization, "exclusive_speaker_diarization"):
        annotation = diarization.exclusive_speaker_diarization
    elif hasattr(diarization, "speaker_diarization"):
        annotation = diarization.speaker_diarization

    speaker_map: dict[str, str] = {}
    segments: list[dict[str, Any]] = []
    for turn, _, label in annotation.itertracks(yield_label=True):
        mapped = speaker_map.setdefault(label, f"Speaker {chr(ord('A') + len(speaker_map))}")
        segments.append(
            {
                "speaker": mapped,
                "start": float(turn.start),
                "end": float(turn.end),
            }
        )

    del pipeline
    del waveform
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    return merge_adjacent_segments(segments, max_gap=0.25)


def merge_adjacent_segments(segments: list[dict[str, Any]], max_gap: float) -> list[dict[str, Any]]:
    if not segments:
        return []
    merged = [segments[0].copy()]
    for current in segments[1:]:
        previous = merged[-1]
        if current["speaker"] == previous["speaker"] and current["start"] - previous["end"] <= max_gap:
            previous["end"] = current["end"]
        else:
            merged.append(current.copy())
    return merged


def transcribe_audio(audio: np.ndarray, model_id: str, hf_home: Path) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    device, compute_type = choose_whisper_settings()
    model_path = resolve_local_model_path(f"Systran/faster-whisper-{model_id}" if "/" not in model_id else model_id, hf_home)
    model = WhisperModel(
        str(model_path),
        device=device,
        compute_type=compute_type,
    )
    word_timestamps_enabled = should_enable_word_timestamps()
    if not word_timestamps_enabled:
        log("中文转写已切换为片段级时间戳模式，以避免 faster-whisper 在 Windows 上的字级时间戳崩溃。")

    segment_iterator, _ = model.transcribe(
        audio,
        language="zh",
        task="transcribe",
        beam_size=5,
        vad_filter=True,
        condition_on_previous_text=True,
        word_timestamps=word_timestamps_enabled,
    )

    words: list[dict[str, Any]] = []
    fallback_segments: list[dict[str, Any]] = []
    for segment in segment_iterator:
        text = normalize_zh_text(segment.text or "")
        if segment.words:
            for word in segment.words:
                if word.start is None or word.end is None:
                    continue
                token = normalize_zh_text(word.word or "")
                if not token:
                    continue
                words.append(
                    {
                        "start": float(word.start),
                        "end": float(word.end),
                        "text": token,
                    }
                )
        elif text:
            fallback_segments.append(
                {
                    "start": float(segment.start),
                    "end": float(segment.end),
                    "text": text,
                }
            )

    del model
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    return words, fallback_segments


def assign_items_to_segments(
    items: list[dict[str, Any]],
    diar_segments: list[dict[str, Any]],
) -> dict[int, list[dict[str, Any]]]:
    assignments: dict[int, list[dict[str, Any]]] = defaultdict(list)
    if not diar_segments:
        return assignments

    seg_index = 0
    for item in items:
        midpoint = (item["start"] + item["end"]) / 2
        while seg_index < len(diar_segments) and midpoint > diar_segments[seg_index]["end"]:
            seg_index += 1
        if seg_index < len(diar_segments):
            segment = diar_segments[seg_index]
            if segment["start"] <= midpoint <= segment["end"]:
                assignments[seg_index].append(item)
                continue

        nearest_index = min(
            range(len(diar_segments)),
            key=lambda idx: min(
                abs(midpoint - diar_segments[idx]["start"]),
                abs(midpoint - diar_segments[idx]["end"]),
            ),
        )
        assignments[nearest_index].append(item)

    return assignments


def build_turns(
    diar_segments: list[dict[str, Any]],
    words: list[dict[str, Any]],
    fallback_segments: list[dict[str, Any]],
) -> list[Turn]:
    turns: list[Turn] = []
    word_assignments = assign_items_to_segments(words, diar_segments)

    for index, diar_segment in enumerate(diar_segments):
        segment_words = word_assignments.get(index, [])
        if segment_words:
            text = normalize_zh_text("".join(item["text"] for item in segment_words))
            if text:
                turns.append(
                    Turn(
                        speaker=diar_segment["speaker"],
                        start=diar_segment["start"],
                        end=diar_segment["end"],
                        text=text,
                    )
                )

    if not turns and fallback_segments:
        fallback_assignments = assign_items_to_segments(fallback_segments, diar_segments)
        for index, diar_segment in enumerate(diar_segments):
            segment_items = fallback_assignments.get(index, [])
            if not segment_items:
                continue
            text = normalize_zh_text(" ".join(item["text"] for item in segment_items))
            if not text:
                continue
            turns.append(
                Turn(
                    speaker=diar_segment["speaker"],
                    start=diar_segment["start"],
                    end=diar_segment["end"],
                    text=text,
                )
            )

    return merge_adjacent_turns(turns, max_gap=0.35)


def merge_adjacent_turns(turns: list[Turn], max_gap: float) -> list[Turn]:
    if not turns:
        return []
    merged = [Turn(**turns[0].__dict__)]
    for current in turns[1:]:
        previous = merged[-1]
        if current.speaker == previous.speaker and current.start - previous.end <= max_gap:
            previous.end = current.end
            previous.text = normalize_zh_text(f"{previous.text} {current.text}")
        else:
            merged.append(Turn(**current.__dict__))
    return merged


class DraftTranslator:
    def __init__(self, model_id: str, hf_home: Path):
        model_path = resolve_local_model_path(model_id, hf_home)
        self.tokenizer = AutoTokenizer.from_pretrained(str(model_path), local_files_only=True)
        self.model = AutoModelForSeq2SeqLM.from_pretrained(str(model_path), local_files_only=True)
        self.model.eval()

    def translate(self, text: str) -> str:
        inputs = self.tokenizer(
            text,
            return_tensors="pt",
            truncation=True,
            max_length=512,
        )
        with torch.inference_mode():
            output = self.model.generate(
                **inputs,
                max_new_tokens=256,
                num_beams=4,
            )
        return cleanup_english(self.tokenizer.decode(output[0], skip_special_tokens=True))


class RefineTranslator:
    def __init__(self, model_id: str, hf_home: Path):
        self.device = "cuda" if torch.cuda.is_available() else "cpu"
        model_path = resolve_local_model_path(model_id, hf_home)
        self.tokenizer = AutoTokenizer.from_pretrained(
            str(model_path),
            trust_remote_code=True,
            local_files_only=True,
        )
        kwargs = build_causal_lm_load_kwargs(model_id, self.device)
        self.model = AutoModelForCausalLM.from_pretrained(str(model_path), **kwargs)
        self.model.eval()
        self.model.generation_config.do_sample = False
        self.model.generation_config.temperature = None
        self.model.generation_config.top_p = None
        self.model.generation_config.top_k = None

    def refine(self, source_text: str, draft_text: str) -> str:
        messages = [
            {"role": "system", "content": SYSTEM_PROMPT},
            {
                "role": "user",
                "content": (
                    "Rewrite the draft English into natural subtitle English.\n"
                    "Chinese source:\n"
                    f"{source_text}\n\n"
                    "Literal draft:\n"
                    f"{draft_text or '(none)'}\n\n"
                    "Return English only."
                ),
            },
        ]
        prompt = apply_chat_template_with_optional_non_thinking(self.tokenizer, messages)
        inputs = self.tokenizer([prompt], return_tensors="pt")
        if self.device == "cuda":
            inputs = {key: value.to("cuda") for key, value in inputs.items()}
        with torch.inference_mode():
            output = self.model.generate(
                **inputs,
                max_new_tokens=196,
                eos_token_id=self.tokenizer.eos_token_id,
                pad_token_id=self.tokenizer.eos_token_id,
            )
        generated = output[0, inputs["input_ids"].shape[1] :]
        refined = cleanup_english(self.tokenizer.decode(generated, skip_special_tokens=True))
        if not refined or contains_cjk(refined) or has_blacklisted_phrase(refined):
            return draft_text or refined
        return refined


class TranscriptReviewer:
    def __init__(self, model_id: str, hf_home: Path):
        self.device = "cuda" if torch.cuda.is_available() else "cpu"
        model_path = resolve_local_model_path(model_id, hf_home)
        self.tokenizer = AutoTokenizer.from_pretrained(
            str(model_path),
            trust_remote_code=True,
            local_files_only=True,
        )
        kwargs = build_causal_lm_load_kwargs(model_id, self.device)
        self.model = AutoModelForCausalLM.from_pretrained(str(model_path), **kwargs)
        self.model.eval()
        self.model.generation_config.do_sample = False
        self.model.generation_config.temperature = None
        self.model.generation_config.top_p = None
        self.model.generation_config.top_k = None

    def summarize_context(self, turns: list[dict[str, Any]]) -> ReviewContext:
        transcript = build_review_context_input(turns)
        messages = [
            {"role": "system", "content": REVIEW_CONTEXT_PROMPT},
            {
                "role": "user",
                "content": (
                    "下面是整篇中文转写，请先提炼中心主旨、事件线和校对重点。\n\n"
                    f"{transcript}\n\n"
                    "请只返回 JSON。"
                ),
            },
        ]
        prompt = apply_chat_template_with_optional_non_thinking(self.tokenizer, messages)
        inputs = self.tokenizer([prompt], return_tensors="pt")
        if self.device == "cuda":
            inputs = {key: value.to("cuda") for key, value in inputs.items()}

        with torch.inference_mode():
            output = self.model.generate(
                **inputs,
                max_new_tokens=320,
                eos_token_id=self.tokenizer.eos_token_id,
                pad_token_id=self.tokenizer.eos_token_id,
            )

        generated = output[0, inputs["input_ids"].shape[1] :]
        decoded = strip_thinking_content(self.tokenizer.decode(generated, skip_special_tokens=True))
        parsed = extract_json_object(decoded) or {}
        summary = normalize_space(str(parsed.get("summary") or ""))
        main_topic = normalize_space(str(parsed.get("main_topic") or parsed.get("topic") or ""))
        event_flow = normalize_string_list(parsed.get("event_flow") or parsed.get("events") or parsed.get("timeline"), 6)
        participants = normalize_string_list(
            parsed.get("participants") or parsed.get("speakers") or parsed.get("entities"),
            6,
        )
        keywords = normalize_string_list(parsed.get("keywords"), 8)
        uncertainties = normalize_string_list(
            parsed.get("uncertainties") or parsed.get("open_questions") or parsed.get("unknowns"),
            4,
        )
        summary = summary[:280]
        main_topic = main_topic[:120]
        if not summary:
            summary = main_topic or "这是一段口播或对话录音，校对时需要结合全文语境判断错别字、病句和具体含义。"
        if not main_topic:
            main_topic = summary

        summary_text = summary
        if keywords:
            summary_text = f"{summary}；关键词：{'、'.join(keywords)}"

        brief_lines = [
            f"中心主旨：{main_topic}",
            f"整体摘要：{summary}",
        ]
        if event_flow:
            brief_lines.append("事件线：")
            brief_lines.extend(f"{idx}. {item}" for idx, item in enumerate(event_flow, start=1))
        if participants:
            brief_lines.append(f"人物/对象：{'、'.join(participants)}")
        if keywords:
            brief_lines.append(f"关键词：{'、'.join(keywords)}")
        if uncertainties:
            brief_lines.append(f"不确定点：{'；'.join(uncertainties)}")
        brief_lines.append("校对要求：每一段都要优先贴合中心主旨，再修正 ASR 错字、病句、术语和断句。")
        return ReviewContext(summary=summary_text, brief="\n".join(brief_lines))

    def review(
        self,
        source_text: str,
        speaker: str,
        start_ts: str,
        end_ts: str,
        global_context: str,
        segment_context: str,
    ) -> tuple[str, list[dict[str, str]]]:
        messages = [
            {"role": "system", "content": REVIEW_SYSTEM_PROMPT},
            {
                "role": "user",
                "content": (
                    "请先把握全文中心主旨，再校对当前这一段。当前段如果原文别扭、不完整或像 ASR 听错，"
                    "请在不脱离全文主旨的前提下，把它改成通顺自然的中文。\n\n"
                    f"全文理解：\n{global_context}\n\n"
                    f"分段上下文：\n{segment_context}\n\n"
                    f"当前说话人：{speaker}\n"
                    f"当前时间：{start_ts} - {end_ts}\n"
                    "原始转写：\n"
                    f"{source_text}\n\n"
                    "请只返回 JSON。"
                ),
            },
        ]
        prompt = apply_chat_template_with_optional_non_thinking(self.tokenizer, messages)
        inputs = self.tokenizer([prompt], return_tensors="pt")
        if self.device == "cuda":
            inputs = {key: value.to("cuda") for key, value in inputs.items()}

        with torch.inference_mode():
            output = self.model.generate(
                **inputs,
                max_new_tokens=320,
                eos_token_id=self.tokenizer.eos_token_id,
                pad_token_id=self.tokenizer.eos_token_id,
            )

        generated = output[0, inputs["input_ids"].shape[1] :]
        decoded = strip_thinking_content(self.tokenizer.decode(generated, skip_special_tokens=True))
        parsed = extract_json_object(decoded) or {}
        candidate_text = str(parsed.get("reviewed_text") or source_text)
        reviewed_text = normalize_review_text(candidate_text, source_text)
        issues = normalize_review_issues(parsed.get("issues", []))
        original_text_normalized = normalize_zh_text(source_text)
        if reviewed_text == original_text_normalized and normalize_zh_text(candidate_text) != original_text_normalized:
            issues = []
        if reviewed_text != original_text_normalized and not issues:
            issues = [
                {
                    "category": "转写修正",
                    "severity": "medium",
                    "source_text": original_text_normalized,
                    "suggestion": reviewed_text,
                    "reason": "AI 认为原句存在转写或表述问题。",
                }
            ]
        return reviewed_text, issues


def build_review_context_input(turns: list[dict[str, Any]], char_limit: int = 6000) -> str:
    rendered = [format_review_turn_line(turn, "zh_text") for turn in turns]
    if not rendered:
        return ""

    total_chars = sum(len(line) + 1 for line in rendered)
    if total_chars <= char_limit:
        return "\n".join(rendered)

    sample_count = max(8, min(len(rendered), max(8, char_limit // 110)))
    if sample_count >= len(rendered):
        selected_indices = list(range(len(rendered)))
    else:
        selected_indices = []
        for slot in range(sample_count):
            index = round(slot * (len(rendered) - 1) / max(1, sample_count - 1))
            if not selected_indices or index != selected_indices[-1]:
                selected_indices.append(index)

    lines: list[str] = []
    used = 0
    previous_index = -1
    for index in selected_indices:
        if previous_index >= 0 and index - previous_index > 1:
            gap_line = f"...（中间省略 {index - previous_index - 1} 段）"
            if used + len(gap_line) + 1 > char_limit:
                break
            lines.append(gap_line)
            used += len(gap_line) + 1
        line = rendered[index]
        if used + len(line) + 1 > char_limit and lines:
            break
        lines.append(line)
        used += len(line) + 1
        previous_index = index
    return "\n".join(lines)


def format_review_turn_line(turn: dict[str, Any], text_key: str) -> str:
    text = normalize_zh_text(str(turn.get(text_key) or ""))
    return f"[{turn['start_ts']} - {turn['end_ts']}] {turn['speaker']}: {text}"


def build_segment_review_context(
    turns: list[dict[str, Any]],
    reviewed_turns: list[dict[str, Any]],
    index: int,
    previous_window: int = 2,
    next_window: int = 2,
) -> str:
    lines: list[str] = []
    previous = reviewed_turns[max(0, len(reviewed_turns) - previous_window) :]
    if previous:
        lines.append("前文（已校对）：")
        lines.extend(format_review_turn_line(turn, "reviewed_text") for turn in previous)

    lines.append("当前段（原始转写）：")
    lines.append(format_review_turn_line(turns[index], "zh_text"))

    following = turns[index + 1 : min(len(turns), index + 1 + next_window)]
    if following:
        lines.append("后文（原始转写）：")
        lines.extend(format_review_turn_line(turn, "zh_text") for turn in following)
    return "\n".join(lines)


def extract_json_object(text: str) -> dict[str, Any] | None:
    cleaned = strip_thinking_content(text)
    cleaned = re.sub(r"^```(?:json)?\s*", "", cleaned, flags=re.IGNORECASE)
    cleaned = re.sub(r"\s*```$", "", cleaned)

    if cleaned.startswith("{") and cleaned.endswith("}"):
        try:
            parsed = json.loads(cleaned)
            if isinstance(parsed, dict):
                return parsed
        except json.JSONDecodeError:
            pass

    start = cleaned.find("{")
    if start < 0:
        return None

    depth = 0
    for index in range(start, len(cleaned)):
        char = cleaned[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                candidate = cleaned[start : index + 1]
                try:
                    parsed = json.loads(candidate)
                    if isinstance(parsed, dict):
                        return parsed
                except json.JSONDecodeError:
                    return None
    return None


def normalize_review_text(reviewed_text: str, original_text: str) -> str:
    original = normalize_zh_text(original_text)
    reviewed = normalize_zh_text(reviewed_text)
    reviewed = re.sub(r"^Speaker\s+[A-Z]\s*:\s*", "", reviewed, flags=re.IGNORECASE)
    reviewed = re.sub(r"[（(][^)）]{0,40}[)）]", "", reviewed)
    reviewed = normalize_zh_text(reviewed)
    if not reviewed:
        return original
    compact_original = re.sub(r"[，。！？；：,.!?;:\s]", "", original)
    compact_reviewed = re.sub(r"[，。！？；：,.!?;:\s]", "", reviewed)
    if not compact_reviewed:
        return original
    if has_role_inversion(original, reviewed):
        return original
    if len(compact_original) <= 6 and len(re.findall(r"[，。！？；：,.!?;:]", reviewed)) > 1:
        return original
    if is_fragment_like(compact_original) and len(compact_reviewed) > max(4, len(compact_original) + 3):
        return original
    if len(compact_original) <= 6 and char_overlap_ratio(compact_original, compact_reviewed) < 0.5:
        return original
    if len(compact_original) <= 6 and len(compact_reviewed) > max(12, len(compact_original) * 2 + 4):
        return original
    if len(compact_original) <= 12 and len(compact_reviewed) > max(22, len(compact_original) * 2 + 8):
        return original
    if len(reviewed) > max(len(original) * 3, len(original) + 60):
        return original
    return reviewed


def is_fragment_like(text: str) -> bool:
    if not text:
        return True
    if text.isdigit():
        return False
    if len(text) <= 2:
        return True
    if len(text) <= 4 and all(char in FRAGMENT_GUARD_CHARS for char in text):
        return True
    return False


def char_overlap_ratio(source_text: str, target_text: str) -> float:
    source_chars = set(source_text)
    if not source_chars:
        return 0.0
    target_chars = set(target_text)
    return len(source_chars & target_chars) / len(source_chars)


def has_role_inversion(original_text: str, reviewed_text: str) -> bool:
    inversion_pairs = (
        ("帮我", "帮你"),
        ("帮你", "帮我"),
        ("给我", "给你"),
        ("给你", "给我"),
        ("我来", "你来"),
        ("你来", "我来"),
        ("我办", "你办"),
        ("你办", "我办"),
    )
    if any(source in original_text and target in reviewed_text for source, target in inversion_pairs):
        return True
    if "帮我" in original_text and re.search(r"我(?:来)?帮[你您]", reviewed_text):
        return True
    if "帮你" in original_text and re.search(r"你(?:来)?帮我", reviewed_text):
        return True
    return False


def normalize_string_list(raw_items: Any, limit: int = 8) -> list[str]:
    if not isinstance(raw_items, list):
        return []

    items: list[str] = []
    for item in raw_items:
        value = normalize_space(str(item))
        if not value or value in items:
            continue
        items.append(value)
        if len(items) >= limit:
            break
    return items


def normalize_review_issues(raw_issues: Any) -> list[dict[str, str]]:
    if not isinstance(raw_issues, list):
        return []

    issues: list[dict[str, str]] = []
    for item in raw_issues[:8]:
        if not isinstance(item, dict):
            continue
        category = normalize_space(str(item.get("category") or "问题"))
        severity = normalize_space(str(item.get("severity") or "medium")).lower()
        if severity not in {"high", "medium", "low"}:
            severity = "medium"
        source_text = normalize_zh_text(str(item.get("source_text") or item.get("source") or ""))
        suggestion = normalize_zh_text(str(item.get("suggestion") or ""))
        reason = normalize_space(str(item.get("reason") or item.get("description") or ""))
        if not any([source_text, suggestion, reason]):
            continue
        issues.append(
            {
                "category": category or "问题",
                "severity": severity,
                "source_text": source_text,
                "suggestion": suggestion,
                "reason": reason or "AI 建议调整此处表述。",
            }
        )
    return issues


def split_sentence_units(text: str) -> list[str]:
    units = re.split(r"(?<=[.!?;:])\s+|(?<=,)\s+", text.strip())
    units = [normalize_space(unit) for unit in units if normalize_space(unit)]
    return units or [normalize_space(text)]


def word_like_count(text: str) -> int:
    return max(1, len(re.findall(r"[A-Za-z0-9']+|[^\w\s]", text)))


def split_long_unit(unit: str) -> list[str]:
    words = unit.split()
    if len(words) <= 14:
        return [unit]
    midpoint = len(words) // 2
    preferred = []
    for index, word in enumerate(words):
        stripped = word.strip(",.;:!?").lower()
        if stripped in {"and", "but", "so", "because", "while", "then", "that", "which"}:
            preferred.append(index)
    if preferred:
        midpoint = min(preferred, key=lambda idx: abs(idx - midpoint))
    left = " ".join(words[: midpoint + 1]).strip()
    right = " ".join(words[midpoint + 1 :]).strip()
    if not left or not right:
        return [unit]
    return [left, right]


def ensure_target_unit_count(units: list[str], target_count: int) -> list[str]:
    refined = list(units)
    while len(refined) < target_count:
        index = max(range(len(refined)), key=lambda idx: word_like_count(refined[idx]))
        parts = split_long_unit(refined[index])
        if len(parts) == 1:
            break
        refined = refined[:index] + parts + refined[index + 1 :]
    while len(refined) > target_count:
        merge_index = min(
            range(len(refined) - 1),
            key=lambda idx: word_like_count(refined[idx]) + word_like_count(refined[idx + 1]),
        )
        merged = normalize_space(f"{refined[merge_index]} {refined[merge_index + 1]}")
        refined = refined[:merge_index] + [merged] + refined[merge_index + 2 :]
    return refined


def choose_segment_count(text: str, duration: float) -> int:
    words = max(1, len(text.split()))
    return max(1, math.ceil(duration / 3.8), math.ceil(words / 10), math.ceil(len(text) / 60))


def split_subtitle_text(text: str, duration: float) -> list[str]:
    target = choose_segment_count(text, duration)
    units = split_sentence_units(text)
    units = ensure_target_unit_count(units, target)
    return [normalize_space(unit) for unit in units if normalize_space(unit)]


def build_segmented_output(turns: list[dict[str, Any]]) -> list[dict[str, Any]]:
    segmented: list[dict[str, Any]] = []
    for turn in turns:
        chunks = split_subtitle_text(turn["en_text"], turn["end"] - turn["start"])
        weights = [word_like_count(chunk) for chunk in chunks]
        total = sum(weights) or len(chunks)
        cursor = turn["start"]
        cumulative = 0
        duration = turn["end"] - turn["start"]
        for chunk_index, chunk in enumerate(chunks):
            cumulative += weights[chunk_index]
            if chunk_index == len(chunks) - 1:
                end = turn["end"]
            else:
                end = turn["start"] + duration * (cumulative / total)
            segmented.append(
                {
                    "speaker": turn["speaker"],
                    "start": round(cursor, 6),
                    "end": round(end, 6),
                    "start_ts": format_ts(cursor),
                    "end_ts": format_ts(end),
                    "original_text": turn["original_text"],
                    "reviewed_text": turn["reviewed_text"],
                    "zh_text": turn["reviewed_text"],
                    "en_text": chunk,
                    "source_turn_index": turn["turn_index"],
                    "segment_index_within_turn": chunk_index,
                }
            )
            cursor = end
    return segmented


def write_txt(path: Path, lines: list[str]) -> None:
    path.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")


def write_review_txt(path: Path, turns: list[dict[str, Any]]) -> None:
    blocks = []
    for turn in turns:
        lines = [
            f"[{turn['start_ts']} - {turn['end_ts']}] {turn['speaker']}",
            f"原始转写: {turn['original_text']}",
            f"AI 校对: {turn['reviewed_text']}",
        ]
        for issue in turn.get("issues", []):
            lines.append(
                f"- [{issue['category']}/{issue['severity']}] {issue['reason']} 建议：{issue['suggestion'] or issue['source_text']}"
            )
        blocks.append("\n".join(lines))
    path.write_text("\n\n".join(blocks).rstrip() + "\n", encoding="utf-8")


def speaker_short_label(speaker: str) -> str:
    match = re.search(r"([A-Z])$", speaker)
    if match:
        return match.group(1)
    return speaker.replace("Speaker ", "S")[:8]


def write_srt(path: Path, segments: list[dict[str, Any]]) -> None:
    blocks = []
    for index, segment in enumerate(segments, start=1):
        speaker_label = speaker_short_label(segment["speaker"])
        text = wrap_subtitle(f"{speaker_label}: {segment['en_text']}")
        blocks.append(
            "\n".join(
                [
                    str(index),
                    f"{format_ts(segment['start'], srt=True)} --> {format_ts(segment['end'], srt=True)}",
                    text,
                ]
            )
        )
    path.write_text("\n\n".join(blocks).rstrip() + "\n", encoding="utf-8")


def build_review_turns(
    turns: list[dict[str, Any]],
    reviewer: TranscriptReviewer,
) -> tuple[list[dict[str, Any]], str]:
    reviewed_turns: list[dict[str, Any]] = []
    total_turns = len(turns)
    emit_progress(0.56, "正在理解全文内容")
    review_context = reviewer.summarize_context(turns)
    emit_progress(0.62, f"正在结合全文语境分析中文稿，共 {total_turns} 段")

    for index, turn in enumerate(turns, start=1):
        segment_context = build_segment_review_context(turns, reviewed_turns, index - 1)
        reviewed_text, issues = reviewer.review(
            turn["zh_text"],
            turn["speaker"],
            turn["start_ts"],
            turn["end_ts"],
            review_context.brief,
            segment_context,
        )
        reviewed_turns.append(
            {
                "turn_index": turn["turn_index"],
                "speaker": turn["speaker"],
                "start": turn["start"],
                "end": turn["end"],
                "start_ts": turn["start_ts"],
                "end_ts": turn["end_ts"],
                "original_text": turn["zh_text"],
                "reviewed_text": reviewed_text,
                "issues": issues,
            }
        )
        emit_progress(0.62 + 0.28 * (index / max(1, total_turns)), f"正在分析第 {index}/{total_turns} 段")

    return reviewed_turns, review_context.summary


def build_review_turns_from_existing(
    review_turns: list[dict[str, Any]],
    reviewer: TranscriptReviewer,
) -> tuple[list[dict[str, Any]], str]:
    turns = [
        {
            "turn_index": turn["turn_index"],
            "speaker": turn["speaker"],
            "start": turn["start"],
            "end": turn["end"],
            "start_ts": turn["start_ts"],
            "end_ts": turn["end_ts"],
            "zh_text": normalize_review_text(turn["reviewed_text"], turn["original_text"]) or turn["original_text"],
            "original_text": turn["original_text"],
        }
        for turn in review_turns
    ]

    reviewed_turns: list[dict[str, Any]] = []
    total_turns = len(turns)
    emit_progress(0.30, "正在理解当前中文稿")
    review_context = reviewer.summarize_context(turns)
    emit_progress(0.38, f"正在基于当前中文稿执行 AI 校对，共 {total_turns} 段")

    for index, turn in enumerate(turns, start=1):
        segment_context = build_segment_review_context(turns, reviewed_turns, index - 1)
        reviewed_text, issues = reviewer.review(
            turn["zh_text"],
            turn["speaker"],
            turn["start_ts"],
            turn["end_ts"],
            review_context.brief,
            segment_context,
        )
        reviewed_turns.append(
            {
                "turn_index": turn["turn_index"],
                "speaker": turn["speaker"],
                "start": turn["start"],
                "end": turn["end"],
                "start_ts": turn["start_ts"],
                "end_ts": turn["end_ts"],
                "original_text": turn["original_text"],
                "reviewed_text": reviewed_text,
                "issues": issues,
            }
        )
        emit_progress(0.38 + 0.52 * (index / max(1, total_turns)), f"正在校对第 {index}/{total_turns} 段")

    return reviewed_turns, review_context.summary


def original_turns_from_review(review_turns: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [
        {
            "turn_index": turn["turn_index"],
            "speaker": turn["speaker"],
            "start": turn["start"],
            "end": turn["end"],
            "start_ts": turn["start_ts"],
            "end_ts": turn["end_ts"],
            "zh_text": turn["original_text"],
        }
        for turn in review_turns
    ]


def write_review_outputs(
    output_dir: Path,
    input_audio: str,
    review_turns: list[dict[str, Any]],
    summary: str,
) -> dict[str, Any]:
    chinese_json = output_dir / "chinese_turns.json"
    chinese_txt = output_dir / "chinese_turns.txt"
    review_json = output_dir / "review_turns.json"
    review_txt = output_dir / "review_turns.txt"
    manifest_path = output_dir / "review_manifest.json"

    original_turns = original_turns_from_review(review_turns)
    save_json(chinese_json, original_turns)
    write_txt(
        chinese_txt,
        [f"[{item['start_ts']} - {item['end_ts']}] {item['speaker']}: {item['zh_text']}" for item in original_turns],
    )
    save_json(review_json, review_turns)
    write_review_txt(review_txt, review_turns)

    manifest = {
        "input_audio": input_audio,
        "output_dir": str(output_dir),
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "turns": len(review_turns),
        "issues": sum(len(turn.get("issues", [])) for turn in review_turns),
        "summary": summary,
        "files": {
            "english_json": "",
            "english_txt": "",
            "english_srt": "",
            "chinese_json": str(chinese_json),
            "chinese_txt": str(chinese_txt),
            "review_json": str(review_json),
            "review_txt": str(review_txt),
        },
    }
    save_json(manifest_path, manifest)
    manifest["manifest"] = str(manifest_path)
    return manifest


def write_translation_outputs(
    output_dir: Path,
    input_audio: str,
    review_turns: list[dict[str, Any]],
    segmented: list[dict[str, Any]],
) -> dict[str, Any]:
    english_json = output_dir / "english_transcript.json"
    english_txt = output_dir / "english_transcript.txt"
    english_srt = output_dir / "english_transcript.srt"
    chinese_json = output_dir / "chinese_turns.json"
    chinese_txt = output_dir / "chinese_turns.txt"
    review_json = output_dir / "review_turns.json"
    review_txt = output_dir / "review_turns.txt"
    manifest_path = output_dir / "result_manifest.json"

    original_turns = original_turns_from_review(review_turns)
    save_json(chinese_json, original_turns)
    write_txt(
        chinese_txt,
        [f"[{item['start_ts']} - {item['end_ts']}] {item['speaker']}: {item['zh_text']}" for item in original_turns],
    )
    save_json(review_json, review_turns)
    write_review_txt(review_txt, review_turns)

    save_json(english_json, segmented)
    write_txt(
        english_txt,
        [f"[{item['start_ts']} - {item['end_ts']}] {item['speaker']}: {item['en_text']}" for item in segmented],
    )
    write_srt(english_srt, segmented)

    manifest = {
        "input_audio": input_audio,
        "output_dir": str(output_dir),
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "turns": len(review_turns),
        "segments": len(segmented),
        "files": {
            "english_json": str(english_json),
            "english_txt": str(english_txt),
            "english_srt": str(english_srt),
            "chinese_json": str(chinese_json),
            "chinese_txt": str(chinese_txt),
            "review_json": str(review_json),
            "review_txt": str(review_txt),
        },
    }
    save_json(manifest_path, manifest)
    manifest["manifest"] = str(manifest_path)
    return manifest


def load_review_turns(path: Path) -> list[dict[str, Any]]:
    turns = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(turns, list):
        raise RuntimeError("AI 校对草稿格式不正确。")

    normalized: list[dict[str, Any]] = []
    for turn in turns:
        if not isinstance(turn, dict):
            continue
        original_text = normalize_zh_text(str(turn.get("original_text") or ""))
        reviewed_text = normalize_review_text(str(turn.get("reviewed_text") or original_text), original_text)
        normalized.append(
            {
                "turn_index": int(turn.get("turn_index", 0)),
                "speaker": str(turn.get("speaker") or "Speaker A"),
                "start": float(turn.get("start", 0.0)),
                "end": float(turn.get("end", 0.0)),
                "start_ts": str(turn.get("start_ts") or format_ts(float(turn.get("start", 0.0)))),
                "end_ts": str(turn.get("end_ts") or format_ts(float(turn.get("end", 0.0)))),
                "original_text": original_text,
                "reviewed_text": reviewed_text,
                "issues": normalize_review_issues(turn.get("issues", [])),
            }
        )
    return normalized


def resolve_input_audio(output_dir: Path, review_json: Path) -> str:
    manifest_path = output_dir / "review_manifest.json"
    if manifest_path.exists():
        try:
            manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
            input_audio = str(manifest.get("input_audio") or "").strip()
            if input_audio:
                return input_audio
        except json.JSONDecodeError:
            pass
    return str(review_json)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="中文音频转英文稿的本地离线处理流程。")
    parser.add_argument("--mode", choices=["review", "proofread", "translate"], default="review")
    parser.add_argument("--input", type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--review-json", type=Path)
    parser.add_argument("--hf-home", default=str(DEFAULT_HF_HOME))
    parser.add_argument("--whisper-model", default=DEFAULT_WHISPER_MODEL)
    parser.add_argument("--draft-model", default=DEFAULT_DRAFT_MODEL)
    parser.add_argument("--refine-model", default=DEFAULT_REFINE_MODEL)
    parser.add_argument("--review-model", default=DEFAULT_REVIEW_MODEL)
    parser.add_argument("--pyannote-model", default=DEFAULT_PYANNOTE_MODEL)
    parser.add_argument("--num-speakers", type=int, default=0)
    args = parser.parse_args()

    if args.mode == "review" and args.input is None:
        parser.error("--input 在 review 模式下必填")
    if args.mode in {"proofread", "translate"} and args.review_json is None:
        parser.error("--review-json 在 translate 模式下必填")
    return args


def configure_environment(hf_home: Path) -> None:
    os.environ["HF_HOME"] = str(hf_home)
    os.environ["HF_HUB_OFFLINE"] = "1"
    os.environ["TRANSFORMERS_OFFLINE"] = "1"
    os.environ.setdefault("PYTHONIOENCODING", "utf-8")


def run_review_mode(args: argparse.Namespace) -> int:
    hf_home = Path(args.hf_home)
    configure_environment(hf_home)

    input_path = args.input.resolve()
    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    staged_input = stage_input_file(input_path, output_dir)

    emit_progress(0.08, "正在解析音频")
    audio = decode_input_audio(staged_input)

    emit_progress(0.20, "正在进行说话人分离")
    diar_segments = diarize_audio(
        audio,
        args.pyannote_model,
        hf_home,
        args.num_speakers if args.num_speakers > 0 else None,
    )
    if not diar_segments:
        raise RuntimeError("说话人分离没有产出任何片段。")

    emit_progress(0.36, "正在转写中文音频")
    words, fallback_segments = transcribe_audio(audio, args.whisper_model, hf_home)
    turns_raw = build_turns(diar_segments, words, fallback_segments)
    if not turns_raw:
        raise RuntimeError("中文转写没有产出任何说话轮次。")

    turns = [
        {
            "turn_index": index,
            "speaker": turn.speaker,
            "start": round(turn.start, 6),
            "end": round(turn.end, 6),
            "start_ts": format_ts(turn.start),
            "end_ts": format_ts(turn.end),
            "zh_text": turn.text,
        }
        for index, turn in enumerate(turns_raw)
    ]

    emit_progress(0.50, "正在加载 AI 校对模型")
    reviewer = TranscriptReviewer(args.review_model, hf_home)
    review_turns, review_summary = build_review_turns(turns, reviewer)

    del reviewer
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    emit_progress(0.94, "正在写出 AI 校对结果")
    manifest = write_review_outputs(output_dir, str(input_path), review_turns, review_summary)
    print(f"REVIEW_MANIFEST={manifest['manifest']}", flush=True)
    return 0


def run_translate_mode(args: argparse.Namespace) -> int:
    hf_home = Path(args.hf_home)
    configure_environment(hf_home)

    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    review_json = args.review_json.resolve()
    review_turns = load_review_turns(review_json)
    if not review_turns:
        raise RuntimeError("没有可用于翻译的校对文本。")

    input_audio = resolve_input_audio(output_dir, review_json)
    emit_progress(0.12, "正在读取校对稿")

    emit_progress(0.24, "正在加载本地翻译模型")
    draft_translator = DraftTranslator(args.draft_model, hf_home)
    refine_translator = RefineTranslator(args.refine_model, hf_home)

    total_turns = len(review_turns)
    translated_turns: list[dict[str, Any]] = []
    emit_progress(0.30, f"正在翻译内容，共 {total_turns} 段")
    for index, turn in enumerate(review_turns, start=1):
        source_text = normalize_review_text(turn["reviewed_text"], turn["original_text"])
        draft = draft_translator.translate(source_text)
        refined = refine_translator.refine(source_text, draft)
        translated_turns.append(
            {
                "turn_index": turn["turn_index"],
                "speaker": turn["speaker"],
                "start": turn["start"],
                "end": turn["end"],
                "start_ts": turn["start_ts"],
                "end_ts": turn["end_ts"],
                "original_text": turn["original_text"],
                "reviewed_text": source_text,
                "en_text": refined or draft or "",
            }
        )
        emit_progress(0.30 + 0.60 * (index / max(1, total_turns)), f"正在翻译第 {index}/{total_turns} 段")

    del draft_translator
    del refine_translator
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    emit_progress(0.94, "正在写出翻译结果")
    segmented = build_segmented_output(translated_turns)
    manifest = write_translation_outputs(output_dir, input_audio, review_turns, segmented)
    print(f"RESULT_MANIFEST={manifest['manifest']}", flush=True)
    return 0


def run_proofread_mode(args: argparse.Namespace) -> int:
    hf_home = Path(args.hf_home)
    configure_environment(hf_home)

    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    review_json = args.review_json.resolve()
    review_turns = load_review_turns(review_json)
    if not review_turns:
        raise RuntimeError("没有可用于 AI 校对的中文稿。")

    input_audio = resolve_input_audio(output_dir, review_json)
    emit_progress(0.12, "正在读取当前中文稿")

    emit_progress(0.20, "正在加载 AI 校对模型")
    reviewer = TranscriptReviewer(args.review_model, hf_home)
    proofread_turns, review_summary = build_review_turns_from_existing(review_turns, reviewer)

    del reviewer
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    emit_progress(0.94, "正在写出 AI 校对结果")
    manifest = write_review_outputs(output_dir, input_audio, proofread_turns, review_summary)
    print(f"REVIEW_MANIFEST={manifest['manifest']}", flush=True)
    return 0


def main() -> int:
    args = parse_args()
    if args.mode == "review":
        return run_review_mode(args)
    if args.mode == "proofread":
        return run_proofread_mode(args)
    if args.mode == "translate":
        return run_translate_mode(args)
    raise RuntimeError(f"不支持的模式: {args.mode}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        log(f"错误: {exc}")
        raise SystemExit(1)
