
GO ?= go
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin

.PHONY: build test clean gestalt gestalt-send install build-frontend

build: gestalt gestalt-send

# Frontend build is required before embedding.
build-frontend:
	cd frontend && npm install && npm run build

gestalt: build-frontend
	$(GO) build -o gestalt ./cmd/gestalt

gestalt-send:
	$(GO) build -o gestalt-send ./cmd/gestalt-send

install: gestalt-send
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send

test:
	go test ./...
	cd frontend && npm test

clean:
	rm -rf frontend/dist
	rm -rf .cache
