package logging

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	lognoop "go.opentelemetry.io/otel/log/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestLoggerWritesToBuffer(t *testing.T) {
	buffer := NewLogBuffer(10)
	logger := NewLoggerWithOutput(buffer, LevelInfo, io.Discard)

	logger.Info("started", map[string]string{"terminal_id": "1"})

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Level != LevelInfo {
		t.Fatalf("expected info level, got %q", entry.Level)
	}
	if entry.Message != "started" {
		t.Fatalf("expected message started, got %q", entry.Message)
	}
	if entry.Context["terminal_id"] != "1" {
		t.Fatalf("expected context terminal_id=1, got %v", entry.Context)
	}
}

func TestLoggerFiltersByLevel(t *testing.T) {
	buffer := NewLogBuffer(10)
	logger := NewLoggerWithOutput(buffer, LevelWarning, io.Discard)

	logger.Info("info", nil)
	logger.Warn("warn", nil)

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != LevelWarning {
		t.Fatalf("expected warning level, got %q", entries[0].Level)
	}
}

func TestLoggerStreamDeliversAllEntries(t *testing.T) {
	logger := NewLoggerWithOutput(NewLogBuffer(50), LevelInfo, io.Discard)
	output, cancel := logger.Subscribe()
	defer cancel()

	const total = 200
	done := make(chan struct{})
	go func() {
		for i := 0; i < total; i++ {
			logger.Info("message", nil)
		}
		close(done)
	}()

	received := 0
	deadline := time.After(2 * time.Second)
	for received < total {
		select {
		case <-output:
			received++
		case <-deadline:
			t.Fatalf("timed out after receiving %d entries", received)
		}
	}

	<-done
}

type testLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (exporter *testLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	for _, record := range records {
		exporter.records = append(exporter.records, record.Clone())
	}
	return nil
}

func (exporter *testLogExporter) Shutdown(context.Context) error {
	return nil
}

func (exporter *testLogExporter) ForceFlush(context.Context) error {
	return nil
}

func (exporter *testLogExporter) snapshot() []sdklog.Record {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	records := make([]sdklog.Record, len(exporter.records))
	copy(records, exporter.records)
	return records
}

func TestLoggerEmitsOTelLogRecord(t *testing.T) {
	exporter := &testLogExporter{}
	processor := sdklog.NewSimpleProcessor(exporter)
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	logglobal.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		logglobal.SetLoggerProvider(lognoop.NewLoggerProvider())
	})

	logger := NewLoggerWithOutput(NewLogBuffer(10), LevelInfo, io.Discard).With(map[string]string{
		"service": "gestalt",
	})
	logger.Info("started", map[string]string{
		"terminal_id": "1",
		"test_id":     "otel-log",
	})

	records := exporter.snapshot()
	var record *sdklog.Record
	for idx := range records {
		entry := &records[idx]
		matched := false
		entry.WalkAttributes(func(attr otellog.KeyValue) bool {
			if attr.Key == "test_id" && attr.Value.Kind() == otellog.KindString && attr.Value.AsString() == "otel-log" {
				matched = true
				return false
			}
			return true
		})
		if matched {
			record = entry
			break
		}
	}
	if record == nil {
		t.Fatalf("expected log record with test_id=otel-log, got %d records", len(records))
	}
	if record.Severity() != otellog.SeverityInfo {
		t.Fatalf("expected severity info, got %v", record.Severity())
	}
	if record.SeverityText() != "info" {
		t.Fatalf("expected severity text info, got %q", record.SeverityText())
	}
	if record.Body().AsString() != "started" {
		t.Fatalf("expected body started, got %q", record.Body().AsString())
	}

	attrs := make(map[string]string)
	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		if attr.Value.Kind() == otellog.KindString {
			attrs[attr.Key] = attr.Value.AsString()
		}
		return true
	})
	if attrs["terminal_id"] != "1" {
		t.Fatalf("expected terminal_id attribute, got %v", attrs["terminal_id"])
	}
	if attrs["service"] != "gestalt" {
		t.Fatalf("expected service attribute, got %v", attrs["service"])
	}
	if attrs["test_id"] != "otel-log" {
		t.Fatalf("expected test_id attribute, got %v", attrs["test_id"])
	}
}
