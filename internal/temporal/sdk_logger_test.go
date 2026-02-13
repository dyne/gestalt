package temporal

import (
	"testing"

	"gestalt/internal/logging"
)

func TestSDKLoggerSuppressesDebug(t *testing.T) {
	buffer := logging.NewLogBuffer(16)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelDebug, nil)
	sdk := newSDKLogger(logger)

	sdk.Debug("debug message", "k", "v")
	sdk.Info("info message", "k", "v")

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "info message" {
		t.Fatalf("expected info log entry, got %q", entries[0].Message)
	}
	if entries[0].Context["gestalt.source"] != "temporal-sdk" {
		t.Fatalf("expected temporal-sdk source, got %q", entries[0].Context["gestalt.source"])
	}
}
