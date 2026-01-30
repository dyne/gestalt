
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
CONFIG_MANIFEST := config/manifest.json
VERSION_INFO := internal/version/build_info.json
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
ARCH := $(UNAME_S)_$(UNAME_M)
DIST := dist
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
CGO ?= 0

.PHONY: build test clean version temporal-dev dev

build: gestalt gestalt-send gestalt-notify
	$(MAKE) -C otel $(ARCH)

# Frontend build is required before embedding.
frontend/dist:
	cd frontend && npm install && VERSION=$(VERSION) npm run build

# Config manifest and version metadata are embedded in the backend binary.
$(CONFIG_MANIFEST) $(VERSION_INFO): scripts/generate-config-manifest.js
	VERSION=$(VERSION) node scripts/generate-config-manifest.js

# make VERSION=1.2.3 to build with specific version
gestalt: frontend/dist $(CONFIG_MANIFEST) $(VERSION_INFO)
	VERSION_LDFLAGS=$$(node scripts/format-version-ldflags.js); \
	$(GO) build -ldflags "$$VERSION_LDFLAGS" -o gestalt ./cmd/gestalt

gestalt-send: $(VERSION_INFO)
	VERSION_LDFLAGS=$$(node scripts/format-version-ldflags.js); \
	$(GO) build  -ldflags "$$VERSION_LDFLAGS" -o gestalt-send ./cmd/gestalt-send

gestalt-notify: $(VERSION_INFO)
	VERSION_LDFLAGS=$$(node scripts/format-version-ldflags.js); \
	$(GO) build  -ldflags "$$VERSION_LDFLAGS" -o gestalt-notify ./cmd/gestalt-notify

install: gestalt gestalt-send gestalt-notify
	install -m 0755 gestalt $(BINDIR)/gestalt
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send
	install -m 0755 gestalt-notify $(BINDIR)/gestalt-notify
	install -m 0755 gestalt-otel $(BINDIR)/gestalt-otel


test:
	go test ./...
	cd frontend && npm test

dev:
	@eval "$$(node scripts/resolve-dev-env.js)"; \
	echo "Starting backend on $$BACKEND_URL and Vite on http://localhost:5173"; \
	trap 'pkill -P $$; exit 0' INT TERM; \
	( $(GO) run ./cmd/gestalt --backend-port $$BACKEND_PORT ) & \
	( cd frontend && GESTALT_BACKEND_URL=$$BACKEND_URL npm run dev ) & \
	wait

temporal-dev:
	temporal server start-dev

version:
	@git describe --tags --always --dirty 2>/dev/null || echo "dev"

clean:
	rm -rf frontend/dist
	rm -rf .cache
	rm -rf gestalt gestalt-send gestalt-notify

release: frontend/dist $(CONFIG_MANIFEST) $(VERSION_INFO)
	@mkdir -p $(DIST)
	@VERSION_LDFLAGS=$$(node scripts/format-version-ldflags.js); \
	$(MAKE) -C otel download-otel OS=linux ARCH=amd64; \
	for p in $(PLATFORMS); do \
		OS=$${p%/*}; ARCH=$${p#*/}; \
		echo "==> building $$OS/$$ARCH"; \
		EXT=""; \
		if [ "$$OS" = "windows" ]; then EXT=".exe"; fi; \
		CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "$$VERSION_LDFLAGS" \
			-o $(DIST)/gestalt$$EXT ./cmd/gestalt; \
		CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "$$VERSION_LDFLAGS" \
			-o $(DIST)/gestalt-send$$EXT ./cmd/gestalt-send; \
		CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "$$VERSION_LDFLAGS" \
			-o $(DIST)/gestalt-notify$$EXT ./cmd/gestalt-notify; \
		cd otel && CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH ./ocb --config builder-config.yaml && cd ..; \
		mv otel/gestalt-otel $(DIST)/gestalt-otel$$EXT; \
		FILES="gestalt$$EXT gestalt-send$$EXT gestalt-notify$$EXT gestalt-otel$$EXT"; \
		tar -czf $(DIST)/gestalt-$$OS-$$ARCH.tar.gz -C $(DIST) $$FILES; \
		cd $(DIST) && rm $$FILES && cd ..; \
	done