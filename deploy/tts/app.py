import io
import logging
import os
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
DEFAULT_SYSTEM_PROMPT = "You are a helpful assistant.<|endofprompt|>"

model = None
model_id = ""
model_dir = Path("/models/Fun-CosyVoice3-0.5B-2512")


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
) -> Response:
    if model is None:
        raise HTTPException(status_code=503, detail="CosyVoice model is still loading")

    normalized_text = text.strip()
    if not normalized_text:
        raise HTTPException(status_code=400, detail="text is required")
    if len(normalized_text) > MAX_TEXT_LENGTH:
        raise HTTPException(status_code=400, detail=f"text must not exceed {MAX_TEXT_LENGTH} characters")

    try:
        # CosyVoice3 performs its own sample-rate conversion and expects a readable
        # file object here, rather than a pre-decoded tensor.
        prompt_wav = prompt_audio.file
        if mode == "zero_shot":
            if not prompt_text or not prompt_text.strip():
                raise HTTPException(status_code=400, detail="prompt_text is required for zero_shot synthesis")
            output = model.inference_zero_shot(
                normalized_text,
                with_system_prompt(prompt_text),
                prompt_wav,
                stream=False,
            )
        else:
            if not instruction or not instruction.strip():
                raise HTTPException(status_code=400, detail="instruction is required for instruct synthesis")
            output = model.inference_instruct2(
                normalized_text,
                with_system_prompt(instruction),
                prompt_wav,
                stream=False,
            )

        chunks = [item["tts_speech"].detach().cpu() for item in output]
        if not chunks:
            raise RuntimeError("CosyVoice returned no audio")
        waveform = torch.cat(chunks, dim=1).squeeze(0).numpy().astype(np.float32, copy=False)
        wav_buffer = io.BytesIO()
        sf.write(wav_buffer, waveform, model.sample_rate, format="WAV", subtype="PCM_16")
    except HTTPException:
        raise
    except Exception as exc:
        logger.exception("CosyVoice synthesis failed")
        raise HTTPException(status_code=500, detail="CosyVoice synthesis failed") from exc
    finally:
        await prompt_audio.close()

    return Response(
        content=wav_buffer.getvalue(),
        media_type="audio/wav",
        headers={
            "Content-Disposition": 'inline; filename="voiceover.wav"',
            "X-AICUT-TTS-Model": model_id,
            "X-AICUT-TTS-Sample-Rate": str(model.sample_rate),
        },
    )
