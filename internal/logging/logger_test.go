package logging

import (
	"io"
	"testing"
	"time"
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
