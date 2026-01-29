package api

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartWebSocketSpanAddsAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otelapi.GetTracerProvider()
	otelapi.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otelapi.SetTracerProvider(previous)
	})

	request := httptest.NewRequest("GET", "http://example.com/ws/logs?token=secret", nil)
	_, span := startWebSocketSpan(request, "/ws/logs", attribute.String("session.id", "t1"))
	span.End()

	spanData := findSpan(recorder.Ended(), wsConnectSpanName, "/ws/logs")
	if spanData == nil {
		t.Fatalf("expected websocket span")
	}

	attrs := spanAttributes(spanData.Attributes())
	if attrs["http.route"] != "/ws/logs" {
		t.Fatalf("expected http.route /ws/logs, got %q", attrs["http.route"])
	}
	if target, ok := attrs["http.target"]; !ok || strings.Contains(target, "token=") {
		t.Fatalf("expected token to be stripped from http.target, got %q", target)
	}
	if attrs["session.id"] != "t1" {
		t.Fatalf("expected session.id t1, got %q", attrs["session.id"])
	}
}

func findSpan(spans []sdktrace.ReadOnlySpan, name, route string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() != name {
			continue
		}
		attrs := spanAttributes(span.Attributes())
		if attrs["http.route"] == route {
			return span
		}
	}
	return nil
}

func spanAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string)
	for _, attr := range attrs {
		values[string(attr.Key)] = attributeValueString(attr.Value)
	}
	return values
}

func attributeValueString(value attribute.Value) string {
	switch value.Type() {
	case attribute.BOOL:
		if value.AsBool() {
			return "true"
		}
		return "false"
	case attribute.INT64:
		return strconv.FormatInt(value.AsInt64(), 10)
	case attribute.FLOAT64:
		return strconv.FormatFloat(value.AsFloat64(), 'g', -1, 64)
	case attribute.STRING:
		return value.AsString()
	default:
		return ""
	}
}
