import asyncio
import os
import shutil
import tempfile
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, File, HTTPException, UploadFile
from funasr import AutoModel


model = None
model_lock = asyncio.Lock()


@asynccontextmanager
async def lifespan(_: FastAPI):
    global model
    model = AutoModel(
        model=os.getenv("ASR_MODEL", "paraformer-zh"),
        vad_model=os.getenv("ASR_VAD_MODEL", "fsmn-vad"),
        punc_model=os.getenv("ASR_PUNC_MODEL", "ct-punc-c"),
        disable_update=True,
    )
    yield
    model = None


app = FastAPI(title="AICut FunASR", lifespan=lifespan)


@app.get("/healthz")
async def healthz():
    if model is None:
        raise HTTPException(status_code=503, detail="model is not ready")
    return {"status": "ok", "model": os.getenv("ASR_MODEL", "paraformer-zh")}


@app.post("/v1/transcriptions")
async def transcribe(file: UploadFile = File(...)):
    if model is None:
        raise HTTPException(status_code=503, detail="model is not ready")

    suffix = Path(file.filename or "audio.wav").suffix or ".wav"
    with tempfile.NamedTemporaryFile(delete=False, suffix=suffix) as temporary_file:
        temporary_path = temporary_file.name
        shutil.copyfileobj(file.file, temporary_file)

    try:
        async with model_lock:
            result = await asyncio.to_thread(
                model.generate,
                input=temporary_path,
                batch_size_s=300,
                sentence_timestamp=True,
            )
    finally:
        os.unlink(temporary_path)

    output = result[0] if result else {}
    return {
        "text": output.get("text", ""),
        "timestamp": output.get("timestamp", []),
        "sentence_info": output.get("sentence_info", []),
    }
