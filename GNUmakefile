
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev
CONFIG_MANIFEST := config/manifest.json
VERSION_INFO := internal/version/build_info.json

.PHONY: build test clean version temporal-dev dev

build: gestalt gestalt-send

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

temporal-dev:
	temporal server start-dev

version:
	@git describe --tags --always --dirty 2>/dev/null || echo "dev"

clean:
	rm -rf frontend/dist
	rm -rf .cache
	rm -rf gestalt gestalt-send
