package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

func TestLoggingMiddlewareAddsCategory(t *testing.T) {
	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelDebug, io.Discard)

	handler := loggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected log entries")
	}
	entry := entries[0]
	if entry.Context["gestalt.category"] != "api" {
		t.Fatalf("expected gestalt.category api, got %q", entry.Context["gestalt.category"])
	}
	if entry.Context["http.route"] != "/api/status" {
		t.Fatalf("expected http.route /api/status, got %q", entry.Context["http.route"])
	}
}

func TestSecurityHeadersOnStatus(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{})
	mux := http.NewServeMux()
	RegisterRoutes(mux, manager, "", StatusConfig{}, "", nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != cacheControlNoStore {
		t.Fatalf("expected Cache-Control %q, got %q", cacheControlNoStore, got)
	}
}

func TestSecurityHeadersOnSPAAssets(t *testing.T) {
	dir := t.TempDir()
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("failed to create assets dir: %v", err)
	}
	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}
	hashedPath := filepath.Join(assetsDir, "app-abcdef12.js")
	if err := os.WriteFile(hashedPath, []byte("console.log('hashed');"), 0o644); err != nil {
		t.Fatalf("failed to write hashed asset: %v", err)
	}
	plainPath := filepath.Join(assetsDir, "app.js")
	if err := os.WriteFile(plainPath, []byte("console.log('plain');"), 0o644); err != nil {
		t.Fatalf("failed to write plain asset: %v", err)
	}

	handler := NewSPAHandler(dir)

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRecorder := httptest.NewRecorder()
	handler.ServeHTTP(indexRecorder, indexReq)
	if got := indexRecorder.Header().Get("Cache-Control"); got != cacheControlNoCache {
		t.Fatalf("expected index Cache-Control %q, got %q", cacheControlNoCache, got)
	}
	if got := indexRecorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected index X-Content-Type-Options nosniff, got %q", got)
	}

	hashedReq := httptest.NewRequest(http.MethodGet, "/assets/app-abcdef12.js", nil)
	hashedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(hashedRecorder, hashedReq)
	if got := hashedRecorder.Header().Get("Cache-Control"); got != cacheControlImmutable {
		t.Fatalf("expected hashed asset Cache-Control %q, got %q", cacheControlImmutable, got)
	}

	plainReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	plainRecorder := httptest.NewRecorder()
	handler.ServeHTTP(plainRecorder, plainReq)
	if got := plainRecorder.Header().Get("Cache-Control"); got != cacheControlNoCache {
		t.Fatalf("expected plain asset Cache-Control %q, got %q", cacheControlNoCache, got)
	}
}
