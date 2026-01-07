
GO ?= go
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin

.PHONY: build test clean

build: gestalt gestalt-send

# Frontend build is required before embedding.
frontend/dist:
	cd frontend && npm install && npm run build

gestalt: frontend/dist
	$(GO) build -o gestalt ./cmd/gestalt

gestalt-send:
	$(GO) build -o gestalt-send ./cmd/gestalt-send

install: gestalt gestalt-send
	install -m 0755 gestalt $(BINDIR)/gestalt
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send

test:
	go test ./...
	cd frontend && npm test

clean:
	rm -rf frontend/dist
	rm -rf .cache
