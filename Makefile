VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w \
           -X github.com/blink-zero/esxport/internal/version.Version=$(VERSION) \
           -X github.com/blink-zero/esxport/internal/version.Commit=$(COMMIT) \
           -X github.com/blink-zero/esxport/internal/version.BuildDate=$(DATE)

.PHONY: build test lint vet fmt clean docker

build:
	go build -ldflags "$(LDFLAGS)" -o bin/esxport ./cmd/esxport

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin/

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(DATE) -t esxport:$(VERSION) .

all: fmt vet lint test build
