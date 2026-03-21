.PHONY: build run clean

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/loadequilibrium ./cmd/loadequilibrium/

run: build
	./bin/loadequilibrium

clean:
	rm -rf bin/
