package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gestalt/internal/otel"
)

func TestHandleOTelLogsRejectsGet(t *testing.T) {
	rest := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/otel/logs?level=info&limit=1", nil)
	resp := httptest.NewRecorder()
	apiErr := rest.handleOTelLogs(resp, req)
	if apiErr == nil {
		t.Fatalf("expected error")
	}
	if apiErr.Status != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", apiErr.Status)
	}
}

func TestHandleOTelLogsRejectsGetWithAllowHeader(t *testing.T) {
	rest := &RestHandler{}
	handler := restHandler("", rest.handleOTelLogs)
	req := httptest.NewRequest(http.MethodGet, "/api/otel/logs", nil)
	resp := httptest.NewRecorder()
	handler(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != "POST" {
		t.Fatalf("expected Allow header POST, got %q", allow)
	}
}

func TestHandleOTelTraces(t *testing.T) {
	dataPath := writeOTelFixture(t)
	otel.SetActiveCollector(otel.CollectorInfo{DataPath: dataPath})
	t.Cleanup(otel.ClearActiveCollector)

	rest := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/otel/traces?trace_id=abc123", nil)
	resp := httptest.NewRecorder()
	if err := rest.handleOTelTraces(resp, req); err != nil {
		t.Fatalf("handleOTelTraces error: %v", err)
	}

	var traces []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(traces))
	}
	if traces[0]["trace_id"] != "abc123" {
		t.Fatalf("expected trace_id abc123, got %v", traces[0]["trace_id"])
	}
	if duration, ok := traces[0]["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("expected duration_ms > 0, got %v", traces[0]["duration_ms"])
	}
}

func TestHandleOTelMetrics(t *testing.T) {
	dataPath := writeOTelFixture(t)
	otel.SetActiveCollector(otel.CollectorInfo{DataPath: dataPath})
	t.Cleanup(otel.ClearActiveCollector)

	rest := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/otel/metrics?name=gestalt.workflow.started", nil)
	resp := httptest.NewRecorder()
	if err := rest.handleOTelMetrics(resp, req); err != nil {
		t.Fatalf("handleOTelMetrics error: %v", err)
	}

	var metrics []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric entry, got %d", len(metrics))
	}
	if metrics[0]["name"] != "gestalt.workflow.started" {
		t.Fatalf("expected metric name gestalt.workflow.started, got %v", metrics[0]["name"])
	}
}

func TestHandleOTelLogsPostValidation(t *testing.T) {
	rest := &RestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/otel/logs", strings.NewReader(`{}`))
	resp := httptest.NewRecorder()
	if err := rest.handleOTelLogs(resp, req); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestHandleOTelLogsPostAccepted(t *testing.T) {
	rest := &RestHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/otel/logs", strings.NewReader(`{"severity_text":"info","body":"hello"}`))
	resp := httptest.NewRecorder()
	if err := rest.handleOTelLogs(resp, req); err != nil {
		t.Fatalf("handleOTelLogs POST error: %v", err)
	}
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", resp.Code)
	}
}

func writeOTelFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")
	lines := []string{
		`{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"gestalt"}}]},"scopeLogs":[{"scope":{"name":"test"},"logRecords":[{"timeUnixNano":"1700000000000000000","severityText":"INFO","body":{"stringValue":"first log"}},{"timeUnixNano":"1700000001000000000","severityText":"ERROR","body":{"stringValue":"second log"}}]}]}]}`,
		`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"gestalt"}}]},"scopeSpans":[{"spans":[{"traceId":"abc123","spanId":"def456","name":"terminal.output","startTimeUnixNano":"1700000002000000000","endTimeUnixNano":"1700000003000000000","status":{"code":2}}]}]}]}`,
		`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"gestalt.workflow.started","sum":{"dataPoints":[{"timeUnixNano":"1700000000000000000","asInt":"3"}]}}]}]}]}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write otel fixture: %v", err)
	}
	return path
}

func logBody(record map[string]any) string {
	body, ok := record["body"].(map[string]any)
	if !ok {
		return ""
	}
	if value, ok := body["stringValue"].(string); ok {
		return value
	}
	return ""
}
