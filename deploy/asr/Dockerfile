ARG ASR_BASE_IMAGE=nvidia/cuda:12.6.3-base-ubuntu22.04
FROM ${ASR_BASE_IMAGE}

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DEFAULT_TIMEOUT=120

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG http_proxy
ARG https_proxy
ARG NO_PROXY
ARG no_proxy
ARG APT_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/ubuntu
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

RUN sed -i "s#http://archive.ubuntu.com/ubuntu#${APT_MIRROR}#g; s#http://security.ubuntu.com/ubuntu#${APT_MIRROR}#g" /etc/apt/sources.list \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        ffmpeg \
        python3 \
        python3-pip \
        python3-setuptools \
        python3-wheel \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" --upgrade pip \
    && python3 -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" \
        "torch==2.5.1" \
        "torchaudio==2.5.1" \
    && python3 -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" \
    "fastapi==0.115.6" \
    "uvicorn[standard]==0.34.0" \
    "python-multipart==0.0.20" \
    "funasr>=1.2.6,<1.3" \
    "modelscope>=1.20,<1.30"

WORKDIR /app

COPY deploy/asr/app.py /app/app.py

EXPOSE 10096

CMD ["python3", "-m", "uvicorn", "app:app", "--host", "0.0.0.0", "--port", "10096"]
