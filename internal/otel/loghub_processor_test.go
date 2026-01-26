package otel

import (
	"testing"
	"time"

	"gestalt/internal/logging"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestSDKRecordToOTLPIncludesRequiredKeys(t *testing.T) {
	var record sdklog.Record
	record.SetTimestamp(time.Unix(100, 0).UTC())
	record.SetObservedTimestamp(time.Unix(101, 0).UTC())
	record.SetSeverity(otellog.SeverityInfo)
	record.SetSeverityText("INFO")
	record.SetBody(otellog.StringValue("hello"))
	record.SetAttributes(otellog.String("gestalt.category", "system"))

	fallback := otlpResourceFromAttributes([]attribute.KeyValue{
		attribute.String("service.name", "gestalt"),
	})
	payload := sdkRecordToOTLP(&record, fallback)
	assertKeys(t, payload, []string{
		"timeUnixNano",
		"observedTimeUnixNano",
		"severityNumber",
		"severityText",
		"body",
		"attributes",
		"resource",
		"scope",
	})

	body := payload["body"].(map[string]any)
	if body["stringValue"] != "hello" {
		t.Fatalf("expected body stringValue, got %v", body["stringValue"])
	}
	resourceMap := payload["resource"].(map[string]any)
	if _, ok := resourceMap["attributes"]; !ok {
		t.Fatalf("expected resource attributes")
	}
}

func TestLegacyEntryToOTLPIncludesRequiredKeys(t *testing.T) {
	entry := logging.LogEntry{
		Timestamp: time.Unix(120, 0).UTC(),
		Level:     logging.LevelWarning,
		Message:   "legacy",
		Context: map[string]string{
			"gestalt.category": "system",
		},
	}

	payload := legacyEntryToOTLP(entry, nil, fallbackScopeName)
	assertKeys(t, payload, []string{
		"timeUnixNano",
		"observedTimeUnixNano",
		"severityNumber",
		"severityText",
		"body",
		"attributes",
		"resource",
		"scope",
	})
	body := payload["body"].(map[string]any)
	if body["stringValue"] != "legacy" {
		t.Fatalf("expected body stringValue, got %v", body["stringValue"])
	}
	if payload["severityNumber"] != severityWarningNumber {
		t.Fatalf("expected warning severity, got %v", payload["severityNumber"])
	}
}

func assertKeys(t *testing.T, payload map[string]any, keys []string) {
	t.Helper()
	if payload == nil {
		t.Fatalf("expected payload")
	}
	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing key %s", key)
		}
	}
}
