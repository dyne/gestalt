package event

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	lognoop "go.opentelemetry.io/otel/log/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

type testOTelExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (exporter *testOTelExporter) Export(_ context.Context, records []sdklog.Record) error {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	for _, record := range records {
		exporter.records = append(exporter.records, record.Clone())
	}
	return nil
}

func (exporter *testOTelExporter) Shutdown(context.Context) error {
	return nil
}

func (exporter *testOTelExporter) ForceFlush(context.Context) error {
	return nil
}

func (exporter *testOTelExporter) snapshot() []sdklog.Record {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	records := make([]sdklog.Record, len(exporter.records))
	copy(records, exporter.records)
	return records
}

func TestBusEmitsOTelLogRecordForEvent(t *testing.T) {
	exporter := &testOTelExporter{}
	processor := sdklog.NewSimpleProcessor(exporter)
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	logglobal.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		logglobal.SetLoggerProvider(lognoop.NewLoggerProvider())
	})

	bus := NewBus[TerminalEvent](context.Background(), BusOptions{Name: "terminal_events"})
	event := NewTerminalEvent("term-1", "terminal_created")
	event.Data = map[string]any{
		"test_id": "otel-event",
		"count":   2,
	}
	bus.Publish(event)

	record := findRecordWithAttribute(exporter.snapshot(), "terminal.data.test_id", "otel-event")
	if record == nil {
		t.Fatalf("expected log record with terminal.data.test_id")
	}
	if record.EventName() != "terminal_created" {
		t.Fatalf("expected event name terminal_created, got %q", record.EventName())
	}

	attrs := recordAttributes(record)
	if attrs["event.bus"] != "terminal_events" {
		t.Fatalf("expected event.bus terminal_events, got %q", attrs["event.bus"])
	}
	if attrs["terminal.id"] != "term-1" {
		t.Fatalf("expected terminal.id term-1, got %q", attrs["terminal.id"])
	}
	if attrs["event.type"] != "terminal_created" {
		t.Fatalf("expected event.type terminal_created, got %q", attrs["event.type"])
	}
}

func TestBusEmitsOTelLogRecordFromFields(t *testing.T) {
	exporter := &testOTelExporter{}
	processor := sdklog.NewSimpleProcessor(exporter)
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	logglobal.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		logglobal.SetLoggerProvider(lognoop.NewLoggerProvider())
	})

	type sampleEvent struct {
		Type      string
		Path      string
		Op        string
		Timestamp time.Time
	}

	bus := NewBus[sampleEvent](context.Background(), BusOptions{Name: "watcher_events"})
	bus.Publish(sampleEvent{
		Type:      "file_changed",
		Path:      "/tmp/plan.org",
		Op:        "WRITE",
		Timestamp: time.Now().UTC(),
	})

	record := findRecordWithAttribute(exporter.snapshot(), "file.path", "/tmp/plan.org")
	if record == nil {
		t.Fatalf("expected log record with file.path")
	}
	if record.EventName() != "file_changed" {
		t.Fatalf("expected event name file_changed, got %q", record.EventName())
	}
}

func findRecordWithAttribute(records []sdklog.Record, key, value string) *sdklog.Record {
	for idx := range records {
		record := &records[idx]
		attrs := recordAttributes(record)
		if attrs[key] == value {
			return record
		}
	}
	return nil
}

func recordAttributes(record *sdklog.Record) map[string]string {
	attrs := make(map[string]string)
	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		switch attr.Value.Kind() {
		case otellog.KindString:
			attrs[attr.Key] = attr.Value.AsString()
		case otellog.KindInt64:
			attrs[attr.Key] = strconv.FormatInt(attr.Value.AsInt64(), 10)
		case otellog.KindFloat64:
			attrs[attr.Key] = strconv.FormatFloat(attr.Value.AsFloat64(), 'g', -1, 64)
		case otellog.KindBool:
			if attr.Value.AsBool() {
				attrs[attr.Key] = "true"
			} else {
				attrs[attr.Key] = "false"
			}
		default:
		}
		return true
	})
	return attrs
}
