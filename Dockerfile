# Multi-stage build: node builds the Vue SPA, Go builds the API binary,
# a minimal final image ships both. CGO disabled → modernc.org/sqlite (pure Go).

# --- stage 1: build the frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- stage 2: build the Go binary ---
FROM golang:1.26-alpine AS go
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built SPA so the binary can serve it from ./web/dist at runtime.
COPY --from=web /web/dist ./web/dist
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/arbuz ./cmd/server

# --- stage 3: minimal runtime ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 arbuz
WORKDIR /app
COPY --from=go /out/arbuz /app/arbuz
COPY --from=web /web/dist /app/web/dist
# Data volume (SQLite db + nothing else secret).
RUN mkdir -p /app/data && chown -R arbuz:arbuz /app
USER arbuz
ENV ARBUZ_LISTEN=:8080 ARBUZ_DB_PATH=/app/data/arbuz.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/arbuz"]
