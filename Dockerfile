FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .

# Zero external deps — no download step needed
RUN go build -ldflags="-s -w" -o loadequilibrium ./cmd/loadequilibrium/

FROM alpine:3.19

RUN addgroup -S le && adduser -S le -G le
WORKDIR /app
COPY --from=builder /build/loadequilibrium .

USER le

EXPOSE 8080

ENTRYPOINT ["./loadequilibrium"]
