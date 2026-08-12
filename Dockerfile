# ── Stage 1: build the React SPA ───────────────────────────────────────
# BASE_REGISTRY lets air-gapped environments use a registry mirror, e.g.
#   docker build --build-arg BASE_REGISTRY=docker.m.daocloud.io/library .
ARG BASE_REGISTRY=docker.io
FROM ${BASE_REGISTRY}/node:22-alpine AS web
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ── Stage 2: build the Go backend ──────────────────────────────────────
FROM ${BASE_REGISTRY}/golang:1.23-alpine AS gobuild
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/portal ./cmd/portal

# ── Stage 3: runtime ───────────────────────────────────────────────────
# Runs as root: the portal is a management control-plane that owns the
# docker socket (create/start/stop/destroy containers). Its own code runs
# unprivileged inside the container; the only escalated surface is the
# socket, which already grants docker-equivalent authority by design.
FROM ${BASE_REGISTRY}/alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl
WORKDIR /app
COPY --from=gobuild /out/portal /app/portal
COPY --from=web /app/dist /app/static
ENV PORTAL_DATA_DIR=/app/data \
    PORTAL_STATIC_DIR=/app/static \
    PORTAL_LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/portal"]
