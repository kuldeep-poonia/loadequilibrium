
# Stage 1 — Build the React UI

FROM node:20-alpine AS ui-builder

WORKDIR /build/ui
COPY ui/package*.json ./
RUN npm ci --prefer-offline

COPY ui/ ./
RUN npm run build
# Output: /build/ui/dist/



# Stage 2 — Build the Go binary

FROM golang:1.22-alpine AS go-builder

WORKDIR /build
# Copy module file first so Docker layer-caches the dependency download.
# This project has zero external Go deps so the layer is tiny.
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o loadequilibrium \
    ./cmd/loadequilibrium/



# Stage 3 — Minimal runtime image
# Single binary + UI assets. No collector sidecar, no nginx, nothing else.

FROM alpine:3.19

# ca-certificates: needed if the collector's HTTP client calls HTTPS endpoints
# wget: used by the healthcheck
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S le && adduser -S le -G le

WORKDIR /app

# The Go binary
COPY --from=go-builder  /build/loadequilibrium .

# The built UI — served from ./ui/ by the Go binary (http.Dir("ui") in server.go)
COPY --from=ui-builder  /build/ui/dist ./ui/

USER le

# Single port. Everything goes through here:
#   GET  /          → UI (React SPA)
#   GET  /ws        → WebSocket live feed
#   GET  /health    → healthcheck
#   GET  /metrics   → Prometheus scrape endpoint
#   POST /api/v1/*  → control & ingest API
EXPOSE 8080

ENTRYPOINT ["./loadequilibrium"]