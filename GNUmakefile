
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev

.PHONY: build test clean version

build: gestalt gestalt-send

# Frontend build is required before embedding.
frontend/dist:
	cd frontend && npm install && VERSION=$(VERSION) npm run build

# make VERSION=1.2.3 to build with specific version
gestalt: frontend/dist
	$(GO) build -ldflags "-X gestalt/internal/version.Version=$(VERSION)" -o gestalt ./cmd/gestalt

gestalt-send:
	$(GO) build  -ldflags "-X gestalt/internal/version.Version=$(VERSION)" -o gestalt-send ./cmd/gestalt-send

install: gestalt gestalt-send
	install -m 0755 gestalt $(BINDIR)/gestalt
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send

test:
	go test ./...
	cd frontend && npm test

version:
	@git describe --tags --always --dirty 2>/dev/null || echo "dev"

clean:
	rm -rf frontend/dist
	rm -rf .cache
	rm -rf gestalt gestalt-send
