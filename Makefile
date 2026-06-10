VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: generate build check test run image

generate:
	go tool templ generate

build: generate
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/warren ./cmd/warren

check:
	go build ./...
	go vet ./...
	go test ./...

test:
	go test ./...

run:
	go run ./cmd/warren serve

image:
	docker build --build-arg VERSION=$(VERSION) -t warren:$(VERSION) .
