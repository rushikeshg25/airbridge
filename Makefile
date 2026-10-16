GO ?= go
NODE ?= node
PYTHON ?= python3
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD)
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build run test check browser-test benchmark release package

build:
	$(GO) build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o bin/airbridge ./cmd/airbridge

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
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o bin/airbridge-linux-amd64 ./cmd/airbridge
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o bin/airbridge-linux-arm64 ./cmd/airbridge
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o bin/airbridge-macos-arm64 ./cmd/airbridge
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags="$(LDFLAGS)" -o bin/airbridge-macos-amd64 ./cmd/airbridge

package: release
	$(PYTHON) scripts/package-release.py $(VERSION)
