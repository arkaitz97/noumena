# Stage 1 — Build frontend (Vue SPA)
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2 — Build Go API
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/noumena-api ./cmd/api

# Stage 3 — Runtime
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

# Required runtime env (no defaults in code — the process refuses to boot
# without them):
#   ENCRYPTION_KEY     — raw AES key, 16/24/32 bytes (mandatory, fail-fast)
#   JWT_SECRET         — >=32 bytes (mandatory, fail-fast)
#   DATABASE_PATH      — SQLite file; mount a persistent volume and point it
#                        at the writable location, e.g. /data/noumena.db
# EXPORT_DIR / EXPORT_STORAGE_DIR default to /var/lib/noumena/* and are
# pre-created below with ownership for the non-root runtime user.

COPY --from=frontend-builder /app/dist ./dist
COPY --from=go-builder /app/noumena-api .

RUN addgroup -S app && adduser -S -G app app \
    && mkdir -p /var/lib/noumena/exports /var/lib/noumena/data-exports /data \
    && chown -R app:app /var/lib/noumena /data /app

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

CMD ["./noumena-api"]