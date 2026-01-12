
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
CONFIG_MANIFEST := config/manifest.json
VERSION_INFO := internal/version/build_info.json

.PHONY: all build test clean clean-desktop version temporal-dev dev wails-install wails-dev \
	frontend-build-desktop gestalt-desktop gestalt-desktop-darwin gestalt-desktop-windows \
	gestalt-desktop-linux

build: gestalt gestalt-send

all: gestalt gestalt-send gestalt-desktop

# Frontend build is required before embedding.
frontend/dist:
	cd frontend && npm install && VERSION=$(VERSION) npm run build

frontend-build-desktop:
	cd frontend && npm run build:desktop

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

install: gestalt gestalt-send
	install -m 0755 gestalt $(BINDIR)/gestalt
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send

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

wails-install:
	$(GO) install github.com/wailsapp/wails/v3/cmd/wails3@latest
	wails3 doctor

wails-dev:
	@command -v wails3 >/dev/null || (echo "Wails not installed. Run: make wails-install" && exit 1)
	wails3 dev -config ./build/config.yml

gestalt-desktop:
	@command -v wails3 >/dev/null || (echo "Wails not installed. Run: make wails-install" && exit 1)
	wails3 build

gestalt-desktop-darwin:
	@command -v wails3 >/dev/null || (echo "Wails not installed. Run: make wails-install" && exit 1)
	wails3 task darwin:build:universal

gestalt-desktop-windows:
	@command -v wails3 >/dev/null || (echo "Wails not installed. Run: make wails-install" && exit 1)
	wails3 task windows:build ARCH=amd64

gestalt-desktop-linux:
	@command -v wails3 >/dev/null || (echo "Wails not installed. Run: make wails-install" && exit 1)
	wails3 task linux:build ARCH=amd64

temporal-dev:
	temporal server start-dev

version:
	@git describe --tags --always --dirty 2>/dev/null || echo "dev"

clean: clean-desktop
	rm -rf frontend/dist
	rm -rf .cache
	rm -rf gestalt gestalt-send

clean-desktop:
	rm -rf bin
	rm -rf frontend/build
	rm -rf frontend/bindings
