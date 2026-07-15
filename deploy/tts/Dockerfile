ARG TTS_BASE_IMAGE=pytorch/pytorch:2.3.1-cuda12.1-cudnn8-runtime
FROM ${TTS_BASE_IMAGE}

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DEFAULT_TIMEOUT=180 \
    PYTHONPATH=/opt/CosyVoice

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG http_proxy
ARG https_proxy
ARG NO_PROXY
ARG no_proxy
ARG APT_MIRROR=http://mirrors.tuna.tsinghua.edu.cn/ubuntu
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG COSYVOICE_REPOSITORY=https://github.com/FunAudioLLM/CosyVoice.git
ARG COSYVOICE_COMMIT=074ca6dc9e80a2f424f1f74b48bdd7d3fea531cc

RUN if [ -n "${APT_MIRROR}" ]; then \
        sed -i "s#http://archive.ubuntu.com/ubuntu#${APT_MIRROR}#g; s#http://security.ubuntu.com/ubuntu#${APT_MIRROR}#g" /etc/apt/sources.list; \
    fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        ffmpeg \
        g++ \
        git \
        libsndfile1 \
        libsox-dev \
        sox \
        unzip \
    && rm -rf /var/lib/apt/lists/*

# Pin the upstream source because CosyVoice3 model layouts and runtime APIs evolve together.
RUN git init /opt/CosyVoice \
    && cd /opt/CosyVoice \
    && git remote add origin "${COSYVOICE_REPOSITORY}" \
    && git fetch --depth 1 origin "${COSYVOICE_COMMIT}" \
    && git checkout --detach FETCH_HEAD \
    && git submodule update --init --recursive --depth 1

# TensorRT and DeepSpeed are optional acceleration/training dependencies. Whisper stays
# installed because the upstream CosyVoice text frontend imports its tokenizer helpers.
RUN cd /opt/CosyVoice \
    && sed -E '/^(deepspeed|openai-whisper|tensorrt-cu12|tensorrt-cu12-bindings|tensorrt-cu12-libs)==/d' requirements.txt > requirements.runtime.txt \
    && python -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" -r requirements.runtime.txt \
    && python -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" "setuptools<81" \
    && python -m pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" --no-build-isolation "openai-whisper==20231117"

WORKDIR /app

COPY deploy/tts/app.py /app/app.py

EXPOSE 50000

CMD ["/bin/sh", "-c", "python -m uvicorn app:app --host 0.0.0.0 --port ${TTS_PORT:-50000}"]
