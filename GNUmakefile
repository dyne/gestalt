
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
CONFIG_MANIFEST := config/manifest.json
VERSION_INFO := internal/version/build_info.json
DIST := dist
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
CGO ?= 0
SCIP_BIN := cmd/gestalt-scip/bin/gestalt-scip

.PHONY: build build-scip test clean version temporal-dev dev

build: gestalt gestalt-send build-scip

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

build-scip:
	@npm i
	@cd cmd/gestalt-scip && npm run build
	@chmod +x cmd/gestalt-scip/bin/gestalt-scip
	@cp cmd/gestalt-scip/bin/gestalt-scip .

install: gestalt gestalt-send build-scip
	install -m 0755 gestalt $(BINDIR)/gestalt
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send
	install -m 0755 gestalt-scip $(BINDIR)/gestalt-scip

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
	rm -rf cmd/gestalt-scip/dist cmd/gestalt-scip/bin
	rm -rf gestalt gestalt-send

release: frontend/dist $(CONFIG_MANIFEST) $(VERSION_INFO) build-scip
	@mkdir -p $(DIST)
	@VERSION_LDFLAGS=$$(node scripts/format-version-ldflags.js); \
	for p in $(PLATFORMS); do \
		OS=$${p%/*}; ARCH=$${p#*/}; \
		echo "==> building $$OS/$$ARCH"; \
		EXT=""; \
		if [ "$$OS" = "windows" ]; then EXT=".exe"; fi; \
		CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "$$VERSION_LDFLAGS" \
			-o $(DIST)/gestalt-$$OS-$$ARCH$$EXT ./cmd/gestalt; \
		CGO_ENABLED=$(CGO) GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "$$VERSION_LDFLAGS" \
			-o $(DIST)/gestalt-send-$$OS-$$ARCH$$EXT ./cmd/gestalt-send; \
		cp $(SCIP_BIN) $(DIST)/gestalt-scip-$$OS-$$ARCH; \
		if [ "$$OS" = "windows" ]; then \
			printf '@echo off\r\nnode \"%%~dp0\\\\gestalt-scip-%s-%s\" %%*\r\n' "$$OS" "$$ARCH" > $(DIST)/gestalt-scip-$$OS-$$ARCH.cmd; \
		fi; \
	done
