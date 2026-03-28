import argparse
import json
import os
import re
import sys
from dataclasses import asdict, dataclass
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve()
SEARCH_ROOTS = [SCRIPT_PATH.parent]
SEARCH_ROOTS.extend(SCRIPT_PATH.parents[:4])


def first_existing_path(candidates: list[Path]) -> Path | None:
    seen: set[str] = set()
    for candidate in candidates:
        if not str(candidate):
            continue
        candidate = candidate.resolve()
        candidate_key = str(candidate)
        if candidate_key in seen:
            continue
        seen.add(candidate_key)
        if candidate.exists():
            return candidate
    return None


def bootstrap_runtime_paths(argv: list[str]) -> tuple[Path | None, Path | None, list[Path]]:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--xtts-site")
    parser.add_argument("--xtts-src")
    parser.add_argument("--extra-site-packages", action="append", default=[])
    args, _ = parser.parse_known_args(argv[1:])

    xtts_site_env = os.environ.get("XTTS_APP_SITE", "").strip()
    xtts_src_env = os.environ.get("XTTS_APP_SRC", "").strip()
    extra_site_env = os.environ.get("XTTS_APP_EXTRA_SITE", "").strip()

    xtts_site = first_existing_path(
        [
            Path(args.xtts_site) if args.xtts_site else Path(),
            Path(xtts_site_env) if xtts_site_env else Path(),
            *[root / "xtts_site" for root in SEARCH_ROOTS],
        ]
    )
    xtts_src = first_existing_path(
        [
            Path(args.xtts_src) if args.xtts_src else Path(),
            Path(xtts_src_env) if xtts_src_env else Path(),
            *[root / "xtts_src" / "TTS-0.22.0" for root in SEARCH_ROOTS],
        ]
    )

    extra_candidates: list[Path] = []
    for item in args.extra_site_packages:
        for chunk in item.split(os.pathsep):
            value = chunk.strip()
            if value:
                extra_candidates.append(Path(value))
    if extra_site_env:
        for chunk in extra_site_env.split(os.pathsep):
            value = chunk.strip()
            if value:
                extra_candidates.append(Path(value))
    for root in SEARCH_ROOTS:
        extra_candidates.append(
            root
            / "sherpa-onnx-streaming-zipformer-zh-xlarge-int8-2025-06-30"
            / ".venv"
            / "Lib"
            / "site-packages"
        )

    resolved_extra: list[Path] = []
    seen_extra: set[str] = set()
    for candidate in extra_candidates:
        candidate = candidate.resolve()
        candidate_key = str(candidate)
        if candidate_key in seen_extra or not candidate.exists():
            continue
        seen_extra.add(candidate_key)
        resolved_extra.append(candidate)

    for extra_path in [xtts_site, xtts_src, *resolved_extra]:
        if extra_path is None:
            continue
        extra_str = str(extra_path)
        if extra_str not in sys.path:
            sys.path.insert(0, extra_str)

    return xtts_site, xtts_src, resolved_extra


BOOTSTRAP_XTTS_SITE, BOOTSTRAP_XTTS_SRC, BOOTSTRAP_EXTRA_SITE = bootstrap_runtime_paths(sys.argv)

import numpy as np
import soundfile as sf
import torch
from TTS.api import TTS


LINE_RE = re.compile(
    r"^\[(?P<start>\d{2}:\d{2}:\d{2}\.\d{3}) - (?P<end>\d{2}:\d{2}:\d{2}\.\d{3})\] "
    r"Speaker (?P<speaker>[A-Z]): (?P<text>.+)$"
)

FEMALE_SPEAKER_PREFS = [
    "Ana Florence",
    "Daisy Studious",
    "Tammie Ema",
    "Claribel Dervla",
    "Gracie Wise",
    "Alison Dietlinde",
    "Sofia Hellen",
    "Lily",
]

MALE_SPEAKER_PREFS = [
    "Andrew Chipper",
    "Damien Black",
    "John Adams",
    "Theo",
    "Craig Gutsy",
    "Arnold",
    "Filipe",
    "Mike",
]

CASUAL_FEMALE_SPEAKER_PREFS = [
    "Daisy Studious",
    "Ana Florence",
    "Tammie Ema",
    "Gracie Wise",
    "Sofia Hellen",
    "Claribel Dervla",
    "Alison Dietlinde",
]

CASUAL_MALE_SPEAKER_PREFS = [
    "Andrew Chipper",
    "Badr Odhiambo",
    "Dionisio Schuyler",
    "Royston Min",
    "Damien Black",
    "John Adams",
]

SENTENCE_SPLIT_RE = re.compile(r"(?<=[.!?])\s+")
ABBREV_END_RE = re.compile(r"(?:\b[A-Z]\.){2,}$")
LEADING_MARKER_RE = re.compile(
    r"^(well|yeah|yep|so|okay|ok|right|honestly|actually|look|hey|you know|i mean|all right)[,.!? ]+",
    re.IGNORECASE,
)
NATURAL_OPENER_RE = re.compile(
    r"^(nah|no|wrong|now|then|actually|yep|yeah|well|look|so|right|okay|ok|got it|true|first time)\b",
    re.IGNORECASE,
)

SPECIAL_CONVERSATIONAL_REWRITES = {
    "Welcome to the Bean Pack AI Podcast.": "Hey... welcome to the Bean Pack AI Podcast.",
    "Let's get started with today's discussion.": "All right... let's get started with today's discussion.",
    "Go ahead.": "Yeah... go ahead.",
    "Uh.": "Uh...",
    "Uh, here.": "Uh... here.",
}

A_ANSWER_PREFIXES = [
    "Well... ",
    "Yeah, so... ",
    "You know, ",
    "Honestly... ",
    "I mean, ",
]

A_CONTINUATION_PREFIXES = [
    "And... ",
    "So... ",
    "I mean, ",
]

B_QUESTION_PREFIXES = [
    "So... ",
    "Okay... ",
    "Right, ",
    "And, ",
]

B_SHORT_PREFIXES = [
    "Yeah... ",
    "Right... ",
]


def emit_progress(progress: float, message: str) -> None:
    bounded = max(0.0, min(progress, 1.0))
    print(f"PROGRESS={bounded:.4f}|{message}", flush=True)


@dataclass
class TranscriptSegment:
    speaker: str
    start: float
    end: float
    text: str


def parse_ts(value: str) -> float:
    hours, minutes, rest = value.split(":")
    seconds, millis = rest.split(".")
    return (
        int(hours) * 3600
        + int(minutes) * 60
        + int(seconds)
        + int(millis) / 1000.0
    )


def normalize_text(text: str) -> str:
    text = text.strip()
    replacements = {
        "\u2019": "'",
        "\u2018": "'",
        "\u201c": '"',
        "\u201d": '"',
        "\u2014": "-",
        "\u2013": "-",
        "閳ユ獨": "'s",
        "閳ユ獧e": "'re",
        "閳ユ獫e": "'ve",
        "閳ユ獟l": "'ll",
        "閳ユ獓": "'d",
        "閳ユ獡": "'m",
        "閳ユ攽": "-p",
        "閳ユ攷": "-m",
    }
    for old, new in replacements.items():
        text = text.replace(old, new)
    text = re.sub(r"\s+", " ", text).strip()
    return text


def parse_transcript(path: Path) -> list[TranscriptSegment]:
    segments: list[TranscriptSegment] = []
    for raw_line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = raw_line.strip()
        if not line:
            continue
        match = LINE_RE.match(line)
        if not match:
            raise ValueError(f"无法识别的文稿行: {line}")
        text = normalize_text(match.group("text"))
        segments.append(
            TranscriptSegment(
                speaker=match.group("speaker"),
                start=parse_ts(match.group("start")),
                end=parse_ts(match.group("end")),
                text=text,
            )
        )
    return segments


def merge_segments(segments: list[TranscriptSegment]) -> list[TranscriptSegment]:
    if not segments:
        return []

    merged: list[TranscriptSegment] = [TranscriptSegment(**asdict(segments[0]))]
    for segment in segments[1:]:
        last = merged[-1]
        if segment.speaker == last.speaker:
            last.end = segment.end
            last.text = normalize_text(f"{last.text} {segment.text}")
            continue
        merged.append(TranscriptSegment(**asdict(segment)))
    return merged


def coalesce_source_segments(
    segments: list[TranscriptSegment],
    min_chars: int = 26,
    max_chars: int = 170,
    max_gap_s: float = 0.18,
) -> list[TranscriptSegment]:
    if not segments:
        return []

    merged: list[TranscriptSegment] = [TranscriptSegment(**asdict(segments[0]))]
    glue_tokens = {
        "and",
        "but",
        "so",
        "now",
        "well",
        "right",
        "okay",
        "ok",
        "yeah",
        "yep",
        "uh",
        "um",
        "actually",
    }

    for segment in segments[1:]:
        last = merged[-1]
        gap_s = max(0.0, segment.start - last.end)
        merged_text = normalize_text(f"{last.text} {segment.text}")
        should_merge = (
            segment.speaker == last.speaker
            and gap_s <= max_gap_s
            and len(merged_text) <= max_chars
            and (
                len(last.text) < min_chars
                or len(segment.text) < 20
                or last.text.endswith((",", ";", ":", "-", "..."))
                or last.text.lower().rstrip(".,!?") in glue_tokens
                or segment.text[:1].islower()
            )
        )
        if should_merge:
            last.end = segment.end
            last.text = merged_text
            continue
        merged.append(TranscriptSegment(**asdict(segment)))
    return merged


def split_turn_text(
    text: str,
    max_chars_per_utterance: int,
    max_sentences_per_utterance: int,
) -> list[str]:
    sentences = [normalize_text(item) for item in SENTENCE_SPLIT_RE.split(text) if item.strip()]
    if not sentences:
        return [normalize_text(text)]

    chunks: list[str] = []
    current: list[str] = []
    for sentence in sentences:
        candidate = normalize_text(" ".join(current + [sentence]))
        if current and (
            len(candidate) > max_chars_per_utterance
            or len(current) >= max_sentences_per_utterance
        ):
            chunks.append(normalize_text(" ".join(current)))
            current = [sentence]
        else:
            current.append(sentence)

    if current:
        chunks.append(normalize_text(" ".join(current)))

    smoothed: list[str] = []
    for chunk in chunks:
        if not smoothed:
            smoothed.append(chunk)
            continue
        if len(smoothed[-1]) < 28 or ABBREV_END_RE.search(smoothed[-1]):
            smoothed[-1] = normalize_text(f"{smoothed[-1]} {chunk}")
            continue
        smoothed.append(chunk)

    return [chunk for chunk in smoothed if chunk]


def expand_turns_for_delivery(
    turns: list[TranscriptSegment],
    max_chars_per_utterance: int,
    max_sentences_per_utterance: int,
) -> list[TranscriptSegment]:
    expanded: list[TranscriptSegment] = []
    for turn in turns:
        chunks = split_turn_text(
            turn.text,
            max_chars_per_utterance=max_chars_per_utterance,
            max_sentences_per_utterance=max_sentences_per_utterance,
        )
        if len(chunks) == 1:
            expanded.append(turn)
            continue

        total_chars = sum(max(len(chunk), 1) for chunk in chunks)
        cursor = turn.start
        duration = max(turn.end - turn.start, 0.0)
        for index, chunk in enumerate(chunks):
            if index == len(chunks) - 1:
                chunk_end = turn.end
            else:
                fraction = len(chunk) / total_chars
                chunk_end = min(turn.end, cursor + duration * fraction)
            expanded.append(
                TranscriptSegment(
                    speaker=turn.speaker,
                    start=cursor,
                    end=chunk_end,
                    text=chunk,
                )
            )
            cursor = chunk_end
    return expanded


def select_reference_segments(
    segments: list[TranscriptSegment],
    speaker: str,
    max_clips: int = 4,
    target_total_seconds: float = 24.0,
) -> list[TranscriptSegment]:
    preferred: list[TranscriptSegment] = []
    fallback: list[TranscriptSegment] = []
    for segment in segments:
        if segment.speaker != speaker:
            continue
        duration = segment.end - segment.start
        if duration >= 2.3 and len(segment.text) >= 28:
            preferred.append(segment)
        elif duration >= 1.2 and len(segment.text) >= 14:
            fallback.append(segment)

    selected: list[TranscriptSegment] = []
    total_seconds = 0.0
    for pool in (
        sorted(preferred, key=lambda item: (item.end - item.start, len(item.text)), reverse=True),
        sorted(fallback, key=lambda item: (item.end - item.start, len(item.text)), reverse=True),
    ):
        for segment in pool:
            if any(
                abs(segment.start - existing.start) < 0.001 and abs(segment.end - existing.end) < 0.001
                for existing in selected
            ):
                continue
            selected.append(segment)
            total_seconds += segment.end - segment.start
            if len(selected) >= max_clips or total_seconds >= target_total_seconds:
                break
        if len(selected) >= max_clips or total_seconds >= target_total_seconds:
            break

    if not selected:
        raise RuntimeError(f"没有为说话人 {speaker} 找到可用的参考片段。")
    return sorted(selected, key=lambda item: item.start)


def extract_reference_clips(
    audio_path: Path,
    segments: list[TranscriptSegment],
    output_dir: Path,
    speaker: str,
    padding_ms: int = 120,
) -> list[Path]:
    audio, sample_rate = sf.read(str(audio_path), dtype="float32")
    if audio.ndim > 1:
        audio = audio.mean(axis=1)

    pad_samples = int(sample_rate * (padding_ms / 1000.0))
    output_dir.mkdir(parents=True, exist_ok=True)
    paths: list[Path] = []
    total_samples = len(audio)

    for index, segment in enumerate(segments, start=1):
        start_sample = max(0, int(segment.start * sample_rate) - pad_samples)
        end_sample = min(total_samples, int(segment.end * sample_rate) + pad_samples)
        clip = audio[start_sample:end_sample]
        ref_path = output_dir / f"{speaker}_{index:02d}_{int(segment.start * 1000)}_{int(segment.end * 1000)}.wav"
        sf.write(str(ref_path), clip, sample_rate, subtype="PCM_16")
        paths.append(ref_path)
    return paths


def build_reference_material(
    reference_audio: Path,
    segments: list[TranscriptSegment],
    refs_dir: Path,
    max_clips: int = 4,
    target_total_seconds: float = 24.0,
) -> tuple[dict[str, list[Path]], dict[str, list[dict[str, float | str]]]]:
    if not reference_audio.exists():
        raise FileNotFoundError(reference_audio)

    details: dict[str, list[dict[str, float | str]]] = {}
    path_map: dict[str, list[Path]] = {}

    for speaker in ("A", "B"):
        selected = select_reference_segments(
            segments,
            speaker=speaker,
            max_clips=max_clips,
            target_total_seconds=target_total_seconds,
        )
        details[speaker] = [
            {
                "start": item.start,
                "end": item.end,
                "duration": round(item.end - item.start, 3),
                "text": item.text,
            }
            for item in selected
        ]
        path_map[speaker] = extract_reference_clips(
            audio_path=reference_audio,
            segments=selected,
            output_dir=refs_dir,
            speaker=speaker,
        )

    return path_map, details


def list_speakers(tts: TTS) -> list[str]:
    speakers = getattr(tts, "speakers", None)
    if isinstance(speakers, dict):
        return list(speakers.keys())
    if isinstance(speakers, (list, tuple)):
        return [str(item) for item in speakers]

    model = getattr(tts.synthesizer, "tts_model", None)
    manager = getattr(model, "speaker_manager", None)
    if manager is None:
        return []
    if hasattr(manager, "speaker_names") and manager.speaker_names:
        return list(manager.speaker_names)
    if hasattr(manager, "speakers") and manager.speakers:
        return list(manager.speakers.keys())
    return []


def choose_speaker(available: list[str], prefs: list[str], exclude: set[str]) -> str:
    for preferred in prefs:
        if preferred in available and preferred not in exclude:
            return preferred
    for candidate in available:
        if candidate not in exclude:
            return candidate
    raise RuntimeError("没有找到可用的 XTTS 预置音色。")


def speaker_prefs_for_style(style: str) -> tuple[list[str], list[str]]:
    if style == "casual-podcast":
        return CASUAL_FEMALE_SPEAKER_PREFS, CASUAL_MALE_SPEAKER_PREFS
    return FEMALE_SPEAKER_PREFS, MALE_SPEAKER_PREFS


def insert_internal_breath_pauses(text: str) -> str:
    replacements = [
        (" but ", ", but "),
        (" because ", ", because "),
        (" especially ", ", especially "),
        (" actually ", ", actually "),
        (" while ", ", while "),
        (" though ", ", though "),
    ]
    updated = text
    for old, new in replacements:
        if old in updated and new not in updated:
            updated = updated.replace(old, new, 1)
            break
    return updated


def humanize_spoken_text(
    turns: list[TranscriptSegment],
    index: int,
    style: str,
    add_conversation_markers: bool,
) -> str:
    text = normalize_text(turns[index].text)
    if style != "casual-podcast" or not add_conversation_markers:
        return text

    if text in SPECIAL_CONVERSATIONAL_REWRITES:
        return SPECIAL_CONVERSATIONAL_REWRITES[text]

    prev_speaker = turns[index - 1].speaker if index > 0 else None
    speaker = turns[index].speaker
    is_question = text.endswith("?")
    has_marker = bool(LEADING_MARKER_RE.match(text))
    has_natural_opener = bool(NATURAL_OPENER_RE.match(text))

    if not has_marker and not has_natural_opener:
        if speaker == "A":
            if prev_speaker == "B" and len(text) >= 28:
                prefix = A_ANSWER_PREFIXES[index % len(A_ANSWER_PREFIXES)]
                text = prefix + text
            elif len(text) >= 70 and index % 3 == 1:
                prefix = A_CONTINUATION_PREFIXES[index % len(A_CONTINUATION_PREFIXES)]
                text = prefix + text
        elif speaker == "B":
            if is_question and len(text) >= 18:
                prefix = B_QUESTION_PREFIXES[index % len(B_QUESTION_PREFIXES)]
                text = prefix + text
            elif len(text) <= 22 and not is_question:
                prefix = B_SHORT_PREFIXES[index % len(B_SHORT_PREFIXES)]
                text = prefix + text.lower()

    if len(text) >= 72:
        text = insert_internal_breath_pauses(text)
    return text


def output_sample_rate(tts: TTS) -> int:
    sample_rate = getattr(tts.synthesizer, "output_sample_rate", None)
    if sample_rate:
        return int(sample_rate)

    tts_config = getattr(tts.synthesizer, "tts_config", None)
    audio_cfg = getattr(tts_config, "audio", None)
    if audio_cfg is not None and hasattr(audio_cfg, "sample_rate"):
        return int(audio_cfg.sample_rate)

    return 24000


def pause_after_turn_ms(
    turns: list[TranscriptSegment],
    index: int,
    turn_pause_ms: int,
    intra_turn_pause_ms: int,
    preserve_timing: bool,
    style: str,
) -> int:
    if index >= len(turns) - 1:
        return 0

    current = turns[index]
    nxt = turns[index + 1]
    if not preserve_timing:
        return intra_turn_pause_ms if nxt.speaker == current.speaker else turn_pause_ms

    natural_gap_ms = max(0, int(round((nxt.start - current.end) * 1000.0)))
    if nxt.speaker == current.speaker:
        floor_ms = 120 if style == "casual-podcast" else 70
        cap_ms = max(intra_turn_pause_ms, 320 if style == "casual-podcast" else 240)
    else:
        floor_ms = 280 if style == "casual-podcast" else 180
        cap_ms = max(turn_pause_ms, 980 if style == "casual-podcast" else 820)
    return max(floor_ms, min(natural_gap_ms, cap_ms))


def synthesize_dialogue(
    turns: list[TranscriptSegment],
    transcript_path: Path,
    output_path: Path,
    manifest_path: Path,
    hf_home: Path,
    coqui_tos_agreed: bool,
    style: str,
    speaker_reference_map: dict[str, list[Path]] | None,
    reference_details: dict[str, list[dict[str, float | str]]] | None,
    reference_audio: Path | None,
    preserve_timing: bool,
    add_conversation_markers: bool,
    female_speaker: str | None,
    male_speaker: str | None,
    language: str,
    pause_ms: int,
    intra_turn_pause_ms: int,
    speed: float,
) -> None:
    os.environ.setdefault("HF_HOME", str(hf_home))
    os.environ.setdefault("HUGGINGFACE_HUB_CACHE", str(hf_home / "hub"))
    os.environ.setdefault("TTS_HOME", str(hf_home / "tts"))
    if coqui_tos_agreed:
        os.environ["COQUI_TOS_AGREED"] = "1"

    device = "cuda" if torch.cuda.is_available() else "cpu"
    print(f"使用设备: {device}", flush=True)
    emit_progress(0.26, "正在加载 XTTS v2")
    tts = TTS(model_name="tts_models/multilingual/multi-dataset/xtts_v2").to(device)
    emit_progress(0.4, "XTTS v2 已就绪")

    reference_map_str = {
        speaker: [str(path) for path in paths]
        for speaker, paths in (speaker_reference_map or {}).items()
        if paths
    }
    use_reference_clone = bool(reference_map_str)
    available_speakers = list_speakers(tts)
    speaker_map: dict[str, str] = {}

    if use_reference_clone:
        print(f"说话人 A -> 参考音频克隆（{len(reference_map_str.get('A', []))} 段）", flush=True)
        print(f"说话人 B -> 参考音频克隆（{len(reference_map_str.get('B', []))} 段）", flush=True)
    else:
        if not available_speakers:
            raise RuntimeError("XTTS 没有暴露可用的预置音色。")
        female_prefs, male_prefs = speaker_prefs_for_style(style)
        chosen_female = female_speaker or choose_speaker(available_speakers, female_prefs, set())
        chosen_male = male_speaker or choose_speaker(available_speakers, male_prefs, {chosen_female})
        speaker_map = {"A": chosen_female, "B": chosen_male}
        print(f"说话人 A -> {chosen_female}", flush=True)
        print(f"说话人 B -> {chosen_male}", flush=True)

    sample_rate = output_sample_rate(tts)
    rendered: list[torch.Tensor] = []
    total_turns = len(turns)

    for index, turn in enumerate(turns, start=1):
        speaker_name = speaker_map.get(turn.speaker, "reference")
        speaker_refs = reference_map_str.get(turn.speaker)
        if not speaker_refs and turn.speaker not in speaker_map:
            raise RuntimeError(f"不支持的说话人标签: {turn.speaker}")
        spoken_text = humanize_spoken_text(
            turns=turns,
            index=index - 1,
            style=style,
            add_conversation_markers=add_conversation_markers,
        )

        print(
            f"[{index}/{total_turns}] 说话人 {turn.speaker}（{speaker_name}）"
            f"{turn.start:.3f}-{turn.end:.3f}: {spoken_text[:90]}",
            flush=True,
        )
        emit_progress(0.4 + (0.48 * index / max(total_turns, 1)), f"正在合成第 {index}/{total_turns} 段")
        if speaker_refs:
            audio = tts.tts(
                text=spoken_text,
                speaker_wav=speaker_refs,
                language=language,
                speed=speed,
                split_sentences=False,
            )
        else:
            audio = tts.tts(
                text=spoken_text,
                speaker=speaker_name,
                language=language,
                speed=speed,
                split_sentences=False,
            )
        waveform = torch.as_tensor(np.asarray(audio), dtype=torch.float32).reshape(1, -1)
        rendered.append(waveform)
        silence_ms = pause_after_turn_ms(
            turns=turns,
            index=index - 1,
            turn_pause_ms=pause_ms,
            intra_turn_pause_ms=intra_turn_pause_ms,
            preserve_timing=preserve_timing,
            style=style,
        )
        if silence_ms > 0:
            silence = torch.zeros(1, int(sample_rate * (silence_ms / 1000.0)), dtype=torch.float32)
            rendered.append(silence)

    if not rendered:
        raise RuntimeError("没有生成任何音频。")

    emit_progress(0.92, "正在写出 WAV 音频")
    final_audio = torch.cat(rendered, dim=1)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    final_audio_np = final_audio.squeeze(0).cpu().numpy()
    sf.write(str(output_path), final_audio_np, sample_rate, subtype="PCM_16")

    manifest = {
        "input_transcript": str(transcript_path),
        "output_audio": str(output_path),
        "generated_with": "xtts_v2",
        "style": style,
        "device": device,
        "sample_rate": sample_rate,
        "pause_ms": pause_ms,
        "intra_turn_pause_ms": intra_turn_pause_ms,
        "preserve_timing": preserve_timing,
        "add_conversation_markers": add_conversation_markers,
        "speed": speed,
        "speakers": speaker_map if speaker_map else {"A": "reference_clone", "B": "reference_clone"},
        "reference_audio": str(reference_audio) if reference_audio else None,
        "speaker_reference_files": {
            speaker: paths for speaker, paths in reference_map_str.items()
        }
        if reference_map_str
        else {},
        "speaker_reference_segments": reference_details or {},
        "available_speakers_sample": available_speakers[:20],
        "turns": len(turns),
        "runtime_paths": {
            "xtts_site": str(BOOTSTRAP_XTTS_SITE) if BOOTSTRAP_XTTS_SITE else None,
            "xtts_src": str(BOOTSTRAP_XTTS_SRC) if BOOTSTRAP_XTTS_SRC else None,
            "extra_site_packages": [str(item) for item in BOOTSTRAP_EXTRA_SITE],
        },
    }
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    emit_progress(0.98, "结果清单已写入")
    print(f"已写出音频: {output_path}", flush=True)
    print(f"已写出清单: {manifest_path}", flush=True)
    print(f"RESULT_MANIFEST={manifest_path}", flush=True)
    emit_progress(1.0, "合成完成")


def main() -> int:
    parser = argparse.ArgumentParser(description="Synthesize a two-speaker English dialogue with XTTS v2.")
    parser.add_argument("--transcript", required=True, help="Path to english_transcript.txt")
    parser.add_argument("--output", help="Output WAV path")
    parser.add_argument("--manifest", help="Output JSON manifest path")
    parser.add_argument("--hf-home", default=r"D:\models\huggingface", help="Hugging Face cache root")
    parser.add_argument("--xtts-site", help="Path to the custom XTTS site-packages overlay")
    parser.add_argument("--xtts-src", help="Path to the patched Coqui TTS source tree")
    parser.add_argument("--extra-site-packages", action="append", default=[], help="Extra Python site-packages path")
    parser.add_argument(
        "--style",
        choices=["default", "casual-podcast"],
        default="default",
        help="Delivery preset.",
    )
    parser.add_argument("--reference-audio", help="Source dialogue audio used for XTTS voice cloning/style transfer")
    parser.add_argument(
        "--preserve-timing",
        action="store_true",
        help="Use transcript timestamps to derive pauses between spoken chunks.",
    )
    parser.add_argument(
        "--add-conversation-markers",
        action="store_true",
        help="Lightly add conversational fillers and punctuation for a more natural read.",
    )
    parser.add_argument(
        "--coqui-tos-agreed",
        action="store_true",
        help="Confirm you agree to Coqui XTTS CPML terms or have a commercial license.",
    )
    parser.add_argument("--female-speaker", help="Override preset speaker for A")
    parser.add_argument("--male-speaker", help="Override preset speaker for B")
    parser.add_argument("--language", default="en", help="XTTS language code")
    parser.add_argument("--pause-ms", type=int, help="Pause between turns")
    parser.add_argument("--intra-turn-pause-ms", type=int, help="Pause between split utterances by the same speaker")
    parser.add_argument("--speed", type=float, help="Speech speed")
    parser.add_argument("--max-chars-per-utterance", type=int, help="Maximum characters per spoken chunk")
    parser.add_argument("--max-sentences-per-utterance", type=int, help="Maximum sentences per spoken chunk")
    args = parser.parse_args()

    transcript_path = Path(args.transcript).resolve()
    if not transcript_path.exists():
        raise FileNotFoundError(transcript_path)

    default_output = transcript_path.with_name("english_dialogue_xttsv2_podcast.wav")
    default_manifest = transcript_path.with_name("english_dialogue_xttsv2_podcast.json")

    output_path = Path(args.output).resolve() if args.output else default_output
    manifest_path = Path(args.manifest).resolve() if args.manifest else default_manifest
    reference_audio = Path(args.reference_audio).resolve() if args.reference_audio else None

    if args.style == "casual-podcast":
        if reference_audio is not None:
            pause_ms = args.pause_ms if args.pause_ms is not None else 430
            intra_turn_pause_ms = args.intra_turn_pause_ms if args.intra_turn_pause_ms is not None else 160
            speed = args.speed if args.speed is not None else 0.98
        else:
            pause_ms = args.pause_ms if args.pause_ms is not None else 520
            intra_turn_pause_ms = args.intra_turn_pause_ms if args.intra_turn_pause_ms is not None else 210
            speed = args.speed if args.speed is not None else 0.94
        max_chars_per_utterance = (
            args.max_chars_per_utterance if args.max_chars_per_utterance is not None else 125
        )
        max_sentences_per_utterance = (
            args.max_sentences_per_utterance if args.max_sentences_per_utterance is not None else 1
        )
    else:
        pause_ms = args.pause_ms if args.pause_ms is not None else 240
        intra_turn_pause_ms = args.intra_turn_pause_ms if args.intra_turn_pause_ms is not None else 120
        speed = args.speed if args.speed is not None else 1.02
        max_chars_per_utterance = (
            args.max_chars_per_utterance if args.max_chars_per_utterance is not None else 220
        )
        max_sentences_per_utterance = (
            args.max_sentences_per_utterance if args.max_sentences_per_utterance is not None else 2
        )

    emit_progress(0.04, "正在读取文稿")
    print(f"读取文稿: {transcript_path}", flush=True)
    raw_segments = parse_transcript(transcript_path)
    speaker_reference_map: dict[str, list[Path]] | None = None
    reference_details: dict[str, list[dict[str, float | str]]] | None = None

    if reference_audio is not None:
        emit_progress(0.12, "正在准备参考片段")
        reference_turns = coalesce_source_segments(
            raw_segments,
            min_chars=34,
            max_chars=120,
            max_gap_s=0.14,
        )
        refs_dir = output_path.parent / "_xtts_reference_clips"
        speaker_reference_map, reference_details = build_reference_material(
            reference_audio=reference_audio,
            segments=reference_turns,
            refs_dir=refs_dir,
        )
        print(
            "参考片段准备完成: "
            f"A={len(speaker_reference_map.get('A', []))}, "
            f"B={len(speaker_reference_map.get('B', []))}",
            flush=True,
        )
        turns = coalesce_source_segments(raw_segments)
        print(f"已将原始文稿整理为 {len(turns)} 个带时间信息的片段", flush=True)
        emit_progress(0.2, "参考音频克隆已准备完成")
    else:
        emit_progress(0.12, "正在整理文稿节奏")
        merged_turns = merge_segments(raw_segments)
        print(f"已合并为 {len(merged_turns)} 个对话轮次", flush=True)
        turns = expand_turns_for_delivery(
            merged_turns,
            max_chars_per_utterance=max_chars_per_utterance,
            max_sentences_per_utterance=max_sentences_per_utterance,
        )
        print(f"已扩展为 {len(turns)} 个朗读片段", flush=True)
        emit_progress(0.2, "朗读片段已准备完成")

    synthesize_dialogue(
        turns=turns,
        transcript_path=transcript_path,
        output_path=output_path,
        manifest_path=manifest_path,
        hf_home=Path(args.hf_home).resolve(),
        coqui_tos_agreed=args.coqui_tos_agreed,
        style=args.style,
        speaker_reference_map=speaker_reference_map,
        reference_details=reference_details,
        reference_audio=reference_audio,
        preserve_timing=args.preserve_timing or reference_audio is not None,
        add_conversation_markers=args.add_conversation_markers,
        female_speaker=args.female_speaker,
        male_speaker=args.male_speaker,
        language=args.language,
        pause_ms=pause_ms,
        intra_turn_pause_ms=intra_turn_pause_ms,
        speed=speed,
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except SystemExit:
        raise
    except Exception as exc:
        print(f"错误: {exc}", file=sys.stderr, flush=True)
        raise
