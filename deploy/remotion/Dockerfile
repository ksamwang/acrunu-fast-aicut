FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    NODE_ENV=production \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    CHROME_PATH=/usr/bin/chromium

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        chromium \
        ffmpeg \
        fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY deploy/remotion/package.json deploy/remotion/package-lock.json ./
RUN npm ci --omit=dev

COPY deploy/remotion/server.mjs ./server.mjs
COPY deploy/remotion/src ./src
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/licenses/acrunu-fast-cut/
RUN chown -R node:node /app

USER node

EXPOSE 3001

CMD ["node", "server.mjs"]
