package otel

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveCollectorBinaryExplicitPath(t *testing.T) {
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "otelcol-gestalt")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	if err := os.WriteFile(binaryPath, []byte("test"), 0o755); err != nil {
		t.Fatalf("write binary failed: %v", err)
	}

	resolved, err := resolveCollectorBinary(binaryPath)
	if err != nil {
		t.Fatalf("resolveCollectorBinary failed: %v", err)
	}
	if resolved != binaryPath {
		t.Fatalf("expected %q, got %q", binaryPath, resolved)
	}
}

func TestResolveCollectorBinaryMissing(t *testing.T) {
	_, err := resolveCollectorBinary("/missing/otelcol-gestalt")
	if err == nil {
		t.Fatalf("expected error for missing binary")
	}
	if err != ErrCollectorNotFound {
		t.Fatalf("expected ErrCollectorNotFound, got %v", err)
	}
}
