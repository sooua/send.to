# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.25
ARG NODE_VERSION=20

# ---------- Stage 1: build the web bundle ----------
FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /web

# Cache dependencies independently of source changes.
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY web/ ./
RUN npm run build


# ---------- Stage 2: build the Go binary ----------
FROM golang:${GO_VERSION}-alpine AS build
RUN apk add --no-cache git ca-certificates mailcap

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
        -tags netgo \
        -ldflags "-s -w -X github.com/sooua/send.to/cmd.Version=$(git describe --tags --always --dirty 2>/dev/null || echo docker) -extldflags '-static'" \
        -trimpath \
        -o /out/sendto .

# Tiny healthcheck binary — lets scratch containers satisfy HEALTHCHECK.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    printf 'package main\nimport ("net/http";"os";"time")\nfunc main(){c:=&http.Client{Timeout:3*time.Second};r,e:=c.Get("http://127.0.0.1:18080/health.html");if e!=nil||r.StatusCode!=200{os.Exit(1)}}\n' > /tmp/healthcheck.go && \
    CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/healthcheck /tmp/healthcheck.go

# Non-root user (UID/GID 10001); overridable at build time.
ARG PUID=10001
ARG PGID=10001
ARG RUNAS=sendto
RUN mkdir -p /out/etc /out/data /out/tmp && \
    echo "${RUNAS}:x:${PUID}:${PGID}::/nonexistent:/sbin/nologin" > /out/etc/passwd && \
    echo "${RUNAS}:x:${PGID}:" > /out/etc/group && \
    chown -R ${PUID}:${PGID} /out/data /out/tmp


# ---------- Stage 3: final minimal image ----------
FROM scratch AS final
LABEL org.opencontainers.image.title="send.to" \
      org.opencontainers.image.description="Minimal file-sharing service" \
      org.opencontainers.image.source="https://github.com/sooua/send.to"

ARG PUID=10001
ARG PGID=10001

COPY --from=build /etc/mime.types /etc/mime.types
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/etc/passwd /etc/passwd
COPY --from=build /out/etc/group /etc/group
COPY --from=build --chown=${PUID}:${PGID} /out/sendto /usr/local/bin/sendto
COPY --from=build --chown=${PUID}:${PGID} /out/healthcheck /usr/local/bin/healthcheck
COPY --from=web   --chown=${PUID}:${PGID} /web/dist /app/web

# Storage + temp dirs created up-front so the non-root user can write to
# them. /data can be bind-mounted; PROVIDER/BASEDIR override via env.
COPY --from=build --chown=${PUID}:${PGID} /out/data /data
COPY --from=build --chown=${PUID}:${PGID} /out/tmp  /tmp

USER ${PUID}:${PGID}

EXPOSE 18080
VOLUME ["/data"]

ENV PROVIDER=local \
    BASEDIR=/data \
    LISTENER=:18080 \
    TEMP_PATH=/tmp \
    WEB_PATH=/app/web

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]

ENTRYPOINT ["/usr/local/bin/sendto"]
