package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAPIMiddlewareRecordsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("test")

	middleware, err := NewAPIInstrumentationMiddleware(meter)
	if err != nil {
		t.Fatalf("middleware init error: %v", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := WithRouteInfo(middleware(handler), RouteInfo{Route: "/api/status", Category: "status", Operation: "read"})

	req := httptest.NewRequest(http.MethodGet, "/api/status?token=secret", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	metrics := collectMetrics(t, reader)
	datapoint := findSumDataPoint(t, metrics, MetricRequestCount)
	attrs := attributeMap(datapoint.Attributes)
	if attrs["http.method"] != http.MethodGet {
		t.Fatalf("expected method attribute, got %q", attrs["http.method"])
	}
	if attrs["http.route"] != "/api/status" {
		t.Fatalf("expected route attribute, got %q", attrs["http.route"])
	}
	if attrs["http.status_code"] != "200" {
		t.Fatalf("expected status code attribute, got %q", attrs["http.status_code"])
	}
	if attrs["http.target"] != "/api/status" {
		t.Fatalf("expected sanitized target, got %q", attrs["http.target"])
	}
}

func TestAPIMiddlewareResolvesAgentAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("test")

	resolver := func(r *http.Request, bodyAgentID string) AgentAttributes {
		return AgentAttributes{
			ID:         "agent-1",
			Name:       "Coder",
			Type:       "codex",
			TerminalID: "123",
		}
	}
	middleware, err := NewAPIInstrumentationMiddleware(meter, WithAPIResolver(resolver))
	if err != nil {
		t.Fatalf("middleware init error: %v", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := WithRouteInfo(middleware(handler), RouteInfo{Route: "/api/sessions/:id", Category: "sessions", Operation: "read"})

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/123", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	metrics := collectMetrics(t, reader)
	datapoint := findSumDataPoint(t, metrics, MetricRequestCount)
	attrs := attributeMap(datapoint.Attributes)
	if attrs["agent.name"] != "Coder" {
		t.Fatalf("expected agent name, got %q", attrs["agent.name"])
	}
	if attrs["agent.type"] != "codex" {
		t.Fatalf("expected agent type, got %q", attrs["agent.type"])
	}
	if attrs["terminal.id"] != "123" {
		t.Fatalf("expected terminal id, got %q", attrs["terminal.id"])
	}
}

func TestAPIMiddlewareRecordsErrors(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("test")

	middleware, err := NewAPIInstrumentationMiddleware(meter)
	if err != nil {
		t.Fatalf("middleware init error: %v", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RecordAPIError(r.Context(), APIErrorInfo{Status: http.StatusBadRequest, Code: "invalid_request", Message: "bad"})
		w.WriteHeader(http.StatusBadRequest)
	})
	wrapped := WithRouteInfo(middleware(handler), RouteInfo{Route: "/api/metrics/summary", Category: "status", Operation: "query"})

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/summary", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	metrics := collectMetrics(t, reader)
	datapoint := findSumDataPoint(t, metrics, MetricAPIErrorCount)
	attrs := attributeMap(datapoint.Attributes)
	if attrs["error_type"] != "invalid_request" {
		t.Fatalf("expected error_type invalid_request, got %q", attrs["error_type"])
	}
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics error: %v", err)
	}
	return rm
}

func findSumDataPoint(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.DataPoint[int64] {
	t.Helper()
	for _, scopeMetrics := range rm.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != name {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) == 0 {
				t.Fatalf("metric %s has no data points", name)
			}
			return sum.DataPoints[0]
		}
	}
	t.Fatalf("metric %s not found", name)
	return metricdata.DataPoint[int64]{}
}

func attributeMap(attrs attribute.Set) map[string]string {
	values := make(map[string]string)
	for index := 0; index < attrs.Len(); index++ {
		kv, ok := attrs.Get(index)
		if !ok {
			continue
		}
		key := string(kv.Key)
		switch kv.Value.Type() {
		case attribute.STRING:
			values[key] = kv.Value.AsString()
		case attribute.INT64:
			values[key] = strconv.FormatInt(kv.Value.AsInt64(), 10)
		case attribute.BOOL:
			values[key] = strconv.FormatBool(kv.Value.AsBool())
		case attribute.FLOAT64:
			values[key] = strconv.FormatFloat(kv.Value.AsFloat64(), 'f', -1, 64)
		default:
			values[key] = kv.Value.Emit()
		}
	}
	return values
}
