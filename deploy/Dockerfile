ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

ARG APP_PATH
ARG BIN_NAME

COPY go.mod go.sum ./
COPY apps ./apps
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/${BIN_NAME} ${APP_PATH}

FROM debian:bookworm-slim

ARG BIN_NAME
ENV BIN_NAME=${BIN_NAME}

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/${BIN_NAME} /app/${BIN_NAME}

CMD ["/bin/sh", "-c", "/app/${BIN_NAME}"]
