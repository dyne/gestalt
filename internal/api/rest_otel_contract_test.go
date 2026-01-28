package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gestalt/internal/otel"
)

func TestHandleOTelLogsAcceptsUIFixture(t *testing.T) {
	payload := otel.LoadUILogFixture(t)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rest := &RestHandler{}
	handler := restHandler("", rest.handleOTelLogs)
	req := httptest.NewRequest(http.MethodPost, "/api/otel/logs", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	handler(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", resp.Code)
	}
}
