FROM pytorch/pytorch:2.5.1-cuda12.4-cudnn9-runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    "fastapi==0.115.6" \
    "uvicorn[standard]==0.34.0" \
    "python-multipart==0.0.20" \
    "funasr>=1.2.6,<1.3" \
    "modelscope>=1.20,<1.30"

WORKDIR /app

COPY deploy/asr/app.py /app/app.py

EXPOSE 10096

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "10096"]
