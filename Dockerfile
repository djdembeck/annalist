FROM oven/bun:alpine AS frontend-builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile

COPY web/src src
COPY web/svelte.config.js web/vite.config.ts web/tsconfig.json ./
RUN bun run build

FROM golang:1.26-alpine AS backend-builder

WORKDIR /build

# Release builds set VERSION via --build-arg VERSION=vX.Y.Z; the ldflags -X
# stamps it into internal/version.Version (reported by `annalist version` and
# /api/health). Dev/CI builds leave it at the default.
ARG VERSION=0.1.0

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /build/web/build /build/web/build

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

# -tags webui embeds the SPA built in the frontend stage (see web/embed.go).
RUN go build -tags webui -ldflags "-s -w -X github.com/djdembeck/annalist/internal/version.Version=${VERSION}" -o /annalist ./cmd/annalist

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata su-exec git

RUN adduser -D -u 1000 annalist

WORKDIR /app

COPY --from=backend-builder /annalist /annalist
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh && \
    mkdir -p /app/data/clones && \
    chown -R annalist:annalist /app /entrypoint.sh

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/annalist", "serve"]
