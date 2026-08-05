# Multi-stage Dockerfile for cmd/api (orchestrator) and cmd/bridge (worker)
# Builds a static binary, runs in a distroless image for tiny attack surface.
#
# Build:
#   docker build -f docker/api.Dockerfile -t niosengine/api:dev .
#   docker build -f docker/api.Dockerfile --build-arg CMD=bridge -t niosengine/bridge:dev .

# ---- build stage ----
FROM golang:1.26-bookworm AS build

ARG CMD=api
ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Cache deps separately for faster rebuilds
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

# Static HTTP probe for the compose healthcheck. distroless has no shell or
# curl, so Docker needs an actual binary to exec.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

# Staged empty dir for the bridge's device-state volume mount point. A named
# volume inherits ownership from the directory that exists in the image, so
# creating it nonroot-owned here is what lets the nonroot runtime user write
# to it. distroless has no shell, so mkdir/chown must happen in this stage.
RUN mkdir -p /state

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

ARG CMD=api
LABEL org.opencontainers.image.title="niosengine-${CMD}"
LABEL org.opencontainers.image.source="https://github.com/amnasahamed/niosengine"

WORKDIR /app
COPY --from=build /out/app /app/app
COPY --from=build /out/healthcheck /app/healthcheck

# cmd/migrate reads goose SQL from $MIGRATIONS_DIR (default "migrations")
# relative to the working dir, so the tree has to ship inside the image.
# Harmless for the api/bridge builds.
COPY --from=build /src/migrations /app/migrations

# Mount point for the bridge_state volume (BRIDGE_STATE_DIR).
COPY --from=build --chown=nonroot:nonroot /state /var/lib/niosengine/bridge

USER nonroot:nonroot
ENTRYPOINT ["/app/app"]
