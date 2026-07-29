import io
import json
import logging
import os
import shutil
import tempfile
import threading
import uuid
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Literal

import numpy as np
import soundfile as sf
import torch
from fastapi import FastAPI, File, Form, HTTPException, UploadFile
from fastapi.responses import JSONResponse, Response
from modelscope import snapshot_download

logging.basicConfig(level=os.environ.get("LOG_LEVEL", "INFO"))
logger = logging.getLogger("aicut.tts")

MAX_TEXT_LENGTH = 4_000
MAX_SYNTHESIS_SEGMENTS = 512
MAX_SEGMENT_PAUSE_MS = 2_000
DEFAULT_SYSTEM_PROMPT = "You are a helpful assistant.<|endofprompt|>"

model = None
model_id = ""
model_dir = Path("/models/Fun-CosyVoice3-0.5B-2512")
model_lock = threading.Lock()


def bool_env(name: str, default: bool) -> bool:
    raw_value = os.environ.get(name)
    if raw_value is None:
        return default
    return raw_value.strip().lower() in {"1", "true", "yes", "on"}


def with_system_prompt(text: str) -> str:
    normalized = text.strip()
    if "<|endofprompt|>" in normalized:
        return normalized
    return DEFAULT_SYSTEM_PROMPT + normalized


def parse_synthesis_segments(raw_value: str | None, full_text: str) -> list[dict[str, int | str]] | None:
    if raw_value is None or not raw_value.strip():
        return None
    try:
        value = json.loads(raw_value)
    except json.JSONDecodeError as exc:
        raise HTTPException(status_code=400, detail="segments_json must be valid JSON") from exc
    if not isinstance(value, list) or not value or len(value) > MAX_SYNTHESIS_SEGMENTS:
        raise HTTPException(
            status_code=400,
            detail=f"segments_json must contain 1 to {MAX_SYNTHESIS_SEGMENTS} segments",
        )

    segments: list[dict[str, int | str]] = []
    for index, item in enumerate(value):
        if not isinstance(item, dict):
            raise HTTPException(status_code=400, detail=f"segment {index + 1} must be an object")
        text = item.get("text")
        pause_after_ms = item.get("pause_after_ms", 0)
        if not isinstance(text, str) or not text.strip():
            raise HTTPException(status_code=400, detail=f"segment {index + 1} text is required")
        if isinstance(pause_after_ms, bool) or not isinstance(pause_after_ms, int):
            raise HTTPException(status_code=400, detail=f"segment {index + 1} pause_after_ms must be an integer")
        if pause_after_ms < 0 or pause_after_ms > MAX_SEGMENT_PAUSE_MS:
            raise HTTPException(
                status_code=400,
                detail=f"segment {index + 1} pause_after_ms must be between 0 and {MAX_SEGMENT_PAUSE_MS}",
            )
        segments.append({"text": text.strip(), "pause_after_ms": pause_after_ms})

    if "".join(str(item["text"]) for item in segments) != full_text:
        raise HTTPException(status_code=400, detail="segments_json text must exactly match text")
    if int(segments[-1]["pause_after_ms"]) != 0:
        raise HTTPException(status_code=400, detail="the final segment must not append a pause")
    return segments


def waveform_from_output(output) -> np.ndarray:
    chunks = [item["tts_speech"].detach().cpu() for item in output]
    if not chunks:
        raise RuntimeError("CosyVoice returned no audio")
    return torch.cat(chunks, dim=1).squeeze(0).numpy().astype(np.float32, copy=False)


def trim_and_fade_waveform(waveform: np.ndarray, sample_rate: int) -> np.ndarray:
    if waveform.ndim != 1 or waveform.size == 0:
        raise RuntimeError("CosyVoice returned an invalid waveform")

    frame_samples = max(1, round(sample_rate * 0.010))
    padding_samples = max(1, round(sample_rate * 0.020))
    frame_rms = []
    for start in range(0, waveform.size, frame_samples):
        frame = waveform[start : start + frame_samples]
        frame_rms.append(float(np.sqrt(np.mean(np.square(frame), dtype=np.float64))))
    peak_rms = max(frame_rms, default=0.0)
    threshold = max(0.0005, peak_rms * 0.015)
    active_frames = [index for index, rms in enumerate(frame_rms) if rms >= threshold]
    if active_frames:
        start = max(0, active_frames[0] * frame_samples - padding_samples)
        end = min(waveform.size, (active_frames[-1] + 1) * frame_samples + padding_samples)
        waveform = waveform[start:end].copy()
    else:
        waveform = waveform.copy()

    fade_samples = min(max(1, round(sample_rate * 0.008)), waveform.size // 2)
    if fade_samples > 0:
        ramp = np.linspace(0.0, 1.0, fade_samples, endpoint=True, dtype=np.float32)
        waveform[:fade_samples] *= ramp
        waveform[-fade_samples:] *= ramp[::-1]
    return waveform


def normalize_synthesis_text(text: str) -> str:
    normalized = model.frontend.text_normalize(text, split=False, text_frontend=True)
    if not isinstance(normalized, str) or not normalized.strip():
        raise RuntimeError("CosyVoice text normalization returned empty text")

    # The upstream frontend turns a trailing comma into a full stop. Each AICUT
    # unit already has an explicit pause, so retain the approved punctuation and
    # avoid changing a continuing clause into sentence-final intonation.
    terminal = text.rstrip()[-1]
    if terminal in "，,、；;：:" and normalized[-1] in "。.!！?？":
        normalized = normalized[:-1] + terminal
    return normalized


def synthesize_segmented_zero_shot(
    segments: list[dict[str, int | str]],
    prompt_text: str,
    prompt_path: Path,
) -> tuple[np.ndarray, list[int], list[int]]:
    speaker_cache_id = f"aicut-request-{uuid.uuid4()}"
    waveforms: list[np.ndarray] = []
    speech_samples: list[int] = []
    unit_samples: list[int] = []

    model.add_zero_shot_spk(with_system_prompt(prompt_text), str(prompt_path), speaker_cache_id)
    try:
        for segment in segments:
            normalized_segment = normalize_synthesis_text(str(segment["text"]))
            output = model.inference_zero_shot(
                normalized_segment,
                "",
                "",
                zero_shot_spk_id=speaker_cache_id,
                stream=False,
                text_frontend=False,
            )
            waveform = trim_and_fade_waveform(waveform_from_output(output), model.sample_rate)
            pause_samples = round(int(segment["pause_after_ms"]) * model.sample_rate / 1000)
            speech_samples.append(int(waveform.size))
            unit_samples.append(int(waveform.size) + pause_samples)
            waveforms.append(waveform)
            if pause_samples > 0:
                waveforms.append(np.zeros(pause_samples, dtype=np.float32))
    finally:
        model.frontend.spk2info.pop(speaker_cache_id, None)

    if not waveforms:
        raise RuntimeError("CosyVoice returned no segmented audio")
    return np.concatenate(waveforms), speech_samples, unit_samples


def ensure_model_downloaded() -> None:
    if (model_dir / "cosyvoice3.yaml").is_file():
        return

    logger.info("downloading CosyVoice model %s into %s", model_id, model_dir)
    model_dir.parent.mkdir(parents=True, exist_ok=True)
    snapshot_download(model_id, local_dir=str(model_dir))
    if not (model_dir / "cosyvoice3.yaml").is_file():
        raise RuntimeError(f"CosyVoice3 model metadata was not found in {model_dir}")


def load_model() -> None:
    global model, model_id, model_dir

    model_id = os.environ.get("COSYVOICE_MODEL_ID", "FunAudioLLM/Fun-CosyVoice3-0.5B-2512")
    model_dir = Path(os.environ.get("COSYVOICE_MODEL_DIR", "/models/Fun-CosyVoice3-0.5B-2512"))
    if not torch.cuda.is_available():
        raise RuntimeError("CosyVoice3 requires a CUDA-enabled container")

    ensure_model_downloaded()
    from cosyvoice.cli.cosyvoice import AutoModel

    logger.info("loading CosyVoice model %s", model_id)
    model = AutoModel(model_dir=str(model_dir), fp16=bool_env("COSYVOICE_FP16", True))
    logger.info("CosyVoice model is ready at sample rate %s", model.sample_rate)


@asynccontextmanager
async def lifespan(_: FastAPI):
    load_model()
    yield


app = FastAPI(title="AICUT CosyVoice3", version="1.0", lifespan=lifespan)


@app.get("/healthz")
def healthz() -> JSONResponse:
    if model is None:
        return JSONResponse(status_code=503, content={"status": "starting"})
    return JSONResponse(
        content={
            "status": "ok",
            "model_id": model_id,
            "sample_rate": model.sample_rate,
            "cuda": torch.cuda.is_available(),
        }
    )


@app.post("/v1/synthesize")
async def synthesize(
    text: str = Form(...),
    prompt_audio: UploadFile = File(...),
    mode: Literal["zero_shot", "instruct"] = Form("zero_shot"),
    prompt_text: str | None = Form(None),
    instruction: str | None = Form(None),
    segments_json: str | None = Form(None),
) -> Response:
    if model is None:
        raise HTTPException(status_code=503, detail="CosyVoice model is still loading")

    normalized_text = text.strip()
    if not normalized_text:
        raise HTTPException(status_code=400, detail="text is required")
    if len(normalized_text) > MAX_TEXT_LENGTH:
        raise HTTPException(status_code=400, detail=f"text must not exceed {MAX_TEXT_LENGTH} characters")
    synthesis_segments = parse_synthesis_segments(segments_json, normalized_text)
    if synthesis_segments is not None and mode != "zero_shot":
        raise HTTPException(status_code=400, detail="segmented synthesis currently requires zero_shot mode")

    prompt_path: Path | None = None
    speech_samples: list[int] = []
    unit_samples: list[int] = []
    try:
        # CosyVoice reads the reference twice for speaker features and speech tokens.
        # Materializing the upload gives both reads a fresh, seekable source.
        suffix = Path(prompt_audio.filename or "prompt.wav").suffix or ".wav"
        with tempfile.NamedTemporaryFile(prefix="aicut-tts-", suffix=suffix, delete=False) as prompt_file:
            shutil.copyfileobj(prompt_audio.file, prompt_file)
            prompt_path = Path(prompt_file.name)

        with model_lock:
            if mode == "zero_shot":
                if not prompt_text or not prompt_text.strip():
                    raise HTTPException(status_code=400, detail="prompt_text is required for zero_shot synthesis")
                if synthesis_segments is not None:
                    waveform, speech_samples, unit_samples = synthesize_segmented_zero_shot(
                        synthesis_segments,
                        prompt_text.strip(),
                        prompt_path,
                    )
                else:
                    output = model.inference_zero_shot(
                        normalized_text,
                        with_system_prompt(prompt_text),
                        str(prompt_path),
                        stream=False,
                    )
                    waveform = waveform_from_output(output)
            else:
                if not instruction or not instruction.strip():
                    raise HTTPException(status_code=400, detail="instruction is required for instruct synthesis")
                output = model.inference_instruct2(
                    normalized_text,
                    with_system_prompt(instruction),
                    str(prompt_path),
                    stream=False,
                )
                waveform = waveform_from_output(output)

        wav_buffer = io.BytesIO()
        sf.write(wav_buffer, waveform, model.sample_rate, format="WAV", subtype="PCM_16")
    except HTTPException:
        raise
    except Exception as exc:
        logger.exception("CosyVoice synthesis failed")
        raise HTTPException(status_code=500, detail="CosyVoice synthesis failed") from exc
    finally:
        await prompt_audio.close()
        if prompt_path is not None:
            prompt_path.unlink(missing_ok=True)

    headers = {
        "Content-Disposition": 'inline; filename="voiceover.wav"',
        "X-AICUT-TTS-Model": model_id,
        "X-AICUT-TTS-Sample-Rate": str(model.sample_rate),
    }
    if unit_samples:
        headers.update(
            {
                "X-AICUT-TTS-Timing-Version": "1",
                "X-AICUT-TTS-Speech-Samples": ",".join(str(value) for value in speech_samples),
                "X-AICUT-TTS-Unit-Samples": ",".join(str(value) for value in unit_samples),
            }
        )

    return Response(
        content=wav_buffer.getvalue(),
        media_type="audio/wav",
        headers=headers,
    )
