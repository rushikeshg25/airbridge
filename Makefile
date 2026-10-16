GO ?= go
NODE ?= node
PYTHON ?= python3

.PHONY: build run test check browser-test benchmark release

build:
	$(GO) build -trimpath -o bin/airbridge ./cmd/airbridge

run: build
	./bin/airbridge

test:
	$(GO) test -race ./...

check: test
	$(GO) vet ./...
	@for file in internal/web/assets/*.js; do $(NODE) --check "$$file" || exit 1; done

browser-test: build
	$(NODE) scripts/browser-smoke.cjs

benchmark: build
	$(PYTHON) scripts/benchmark.py --mib 1024

release:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o bin/airbridge-linux-amd64 ./cmd/airbridge
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o bin/airbridge-linux-arm64 ./cmd/airbridge
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o bin/airbridge-macos-arm64 ./cmd/airbridge
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o bin/airbridge-macos-amd64 ./cmd/airbridge
