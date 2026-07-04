BINARY ?= wiim
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/zzwong/wiim-cli/internal/wiim.Version=$(VERSION)

.PHONY: test build install clean

test:
	go test ./...

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/wiim

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/wiim

clean:
	rm -f $(BINARY)
