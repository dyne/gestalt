
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
VERSION ?= dev

.PHONY: build test clean version temporal-dev dev

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

dev:
	@BACKEND_PORT=$${GESTALT_PORT}; \
	BACKEND_URL=$${GESTALT_BACKEND_URL}; \
	if [ -n "$$BACKEND_URL" ] && [ -z "$$BACKEND_PORT" ]; then \
		BACKEND_PORT=$$(python3 -c 'import os,urllib.parse; url=os.environ.get("GESTALT_BACKEND_URL",""); parsed=urllib.parse.urlparse(url); port=parsed.port; print(port if port is not None else (443 if parsed.scheme=="https" else 80))'); \
	fi; \
	if [ -z "$$BACKEND_PORT" ]; then \
		BACKEND_PORT=$$(python3 -c 'import socket; sock=socket.socket(); sock.bind(("",0)); print(sock.getsockname()[1]); sock.close()'); \
	fi; \
	if [ -z "$$BACKEND_URL" ]; then \
		BACKEND_URL=http://localhost:$$BACKEND_PORT; \
	fi; \
	echo "Starting backend on $$BACKEND_URL and Vite on http://localhost:5173"; \
	trap 'pkill -P $$; exit 0' INT TERM; \
	( $(GO) run ./cmd/gestalt --port $$BACKEND_PORT ) & \
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
