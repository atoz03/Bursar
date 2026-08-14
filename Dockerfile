# Bursar controller image.
#
# Only the controller is containerised. The node agent must run directly on each
# compute host: it needs cgroup v2 write access, systemd, the host SSH state, and
# the NVIDIA driver stack. See docs/node-agent.md.

# ---------- Stage 1: build the Web UI ----------
FROM node:20.20.0-bookworm-slim AS web
WORKDIR /src/web

RUN npm install --global pnpm@10.28.2

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# ---------- Stage 2: build the controller ----------
FROM golang:1.26.6-bookworm AS build
WORKDIR /src

COPY controller/go.mod controller/go.sum ./controller/
RUN cd controller && go mod download

COPY controller/ ./controller/
RUN cd controller \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/controller .

# ---------- Stage 3: runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/controller /app/controller
COPY --from=web /src/web/dist /app/web/dist
COPY database/migrations /app/database/migrations
COPY config/controller.yaml /app/config/controller.yaml

# Secrets and the DSN come from the environment (see docs/installation.md);
# the bundled YAML only supplies non-secret defaults.
ENV GPUOPS_LISTEN_ADDR=0.0.0.0:8080 \
    GPUOPS_MIGRATION_DIR=/app/database/migrations \
    GPUOPS_WEB_DIR=/app/web/dist \
    CONTROLLER_CONFIG=/app/config/controller.yaml

EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/app/controller"]
