package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gestalt/internal/otel"
)

func TestMetricsSummaryEndpoint(t *testing.T) {
	store := otel.NewAPISummaryStore()
	store.Record(otel.APISample{Route: "/api/status", Category: "status", AgentName: "alpha", DurationSeconds: 0.2, HasError: false})
	handler := &RestHandler{MetricsSummary: store}

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/summary", nil)
	rec := httptest.NewRecorder()
	restHandler("", handler.handleMetricsSummary)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var summary otel.MetricsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at")
	}
	if len(summary.TopEndpoints) == 0 || summary.TopEndpoints[0].Route != "/api/status" {
		t.Fatalf("unexpected top endpoints: %#v", summary.TopEndpoints)
	}
}

func TestMetricsSummaryEndpointUnavailable(t *testing.T) {
	handler := &RestHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/summary", nil)
	rec := httptest.NewRecorder()
	restHandler("", handler.handleMetricsSummary)(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestMetricsSummaryEndpointMethodNotAllowed(t *testing.T) {
	store := otel.NewAPISummaryStore()
	handler := &RestHandler{MetricsSummary: store}
	req := httptest.NewRequest(http.MethodPost, "/api/metrics/summary", nil)
	rec := httptest.NewRecorder()
	restHandler("", handler.handleMetricsSummary)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow GET, got %q", allow)
	}
}

func TestMetricsSummaryCacheTTL(t *testing.T) {
	store := otel.NewAPISummaryStore()
	store.Record(otel.APISample{Route: "/api/status", Category: "status", AgentName: "alpha", DurationSeconds: 0.2, HasError: false})

	first := store.Summary(time.Now().UTC())
	second := store.Summary(first.UpdatedAt.Add(30 * time.Second))
	if !first.UpdatedAt.Equal(second.UpdatedAt) {
		t.Fatalf("expected cached summary within TTL")
	}
}
