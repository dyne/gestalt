//go:build noscip

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSCIPDisabledHandler(t *testing.T) {
	handler, err := NewSCIPHandler("index.db", nil, SCIPHandlerOptions{})
	if err != nil {
		t.Fatalf("expected handler, got error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/scip/index", nil)
	res := httptest.NewRecorder()
	restHandler("", handler.ReIndex)(res, req)

	if res.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", http.StatusNotImplemented, res.Code)
	}

	var payload errorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "scip_disabled" {
		t.Fatalf("expected error code scip_disabled, got %q", payload.Code)
	}
}
