ARG NODE_VERSION=22

FROM node:${NODE_VERSION}-bookworm-slim AS builder

WORKDIR /src/apps/web

COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci

COPY apps/web ./
RUN npm run build

FROM nginx:1.27-alpine

COPY deploy/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/apps/web/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=15s --timeout=3s --retries=5 \
  CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
