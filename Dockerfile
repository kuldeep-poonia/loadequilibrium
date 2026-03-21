FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/loadequilibrium ./cmd/loadequilibrium/

FROM scratch
COPY --from=builder /out/loadequilibrium /loadequilibrium
EXPOSE 8080
ENTRYPOINT ["/loadequilibrium"]
