.PHONY: build run clean

GO ?= go
BIN_DIR ?= bin
BINARY ?= loadequilibrium

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY) ./cmd/loadequilibrium/

run:
	$(GO) run ./cmd/loadequilibrium/

clean:
	rm -rf $(BIN_DIR)/
