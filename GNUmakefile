
GO ?= go
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin

.PHONY: build test clean gestalt gestalt-send install

build: gestalt gestalt-send
	cd frontend && npm install && npm run build

gestalt:
	$(GO) build -o gestalt cmd/gestalt/main.go

gestalt-send:
	$(GO) build -o gestalt-send cmd/gestalt-send/main.go

install: gestalt-send
	install -m 0755 gestalt-send $(BINDIR)/gestalt-send

test:
	go test ./...
	cd frontend && npm test

clean:
	rm -rf frontend/dist
	rm -rf .cache
