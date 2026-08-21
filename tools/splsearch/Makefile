GO ?= go
BIN ?= bin/splsearch
GO_LDFLAGS ?= -linkmode=external

.PHONY: build test test-cover playwright-install

build:
	mkdir -p $(dir $(BIN))
	$(GO) build -ldflags="$(GO_LDFLAGS)" -o $(BIN) ./cmd/splsearch
	@if [ "$$(uname -s)" = "Darwin" ] && command -v codesign >/dev/null 2>&1; then codesign --force -s - $(BIN) >/dev/null; fi

test:
	$(GO) test -ldflags="$(GO_LDFLAGS)" ./...

test-cover:
	$(GO) test -ldflags="$(GO_LDFLAGS)" -cover ./...

playwright-install:
	$(GO) run -ldflags="$(GO_LDFLAGS)" github.com/playwright-community/playwright-go/cmd/playwright install chromium
