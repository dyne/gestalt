package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gestalt/internal/logging"
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
