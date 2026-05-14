FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .

# Zero external deps — no download step needed
RUN go build -ldflags="-s -w" -o loadequilibrium ./cmd/loadequilibrium/

FROM node:20-alpine AS ui-builder

WORKDIR /build/ui
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

FROM alpine:3.19

RUN addgroup -S le && adduser -S le -G le
WORKDIR /app
COPY --from=builder /build/loadequilibrium .
COPY --from=ui-builder /build/ui/dist ./ui/

USER le

EXPOSE 8080

ENTRYPOINT ["./loadequilibrium"]
