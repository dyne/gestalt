package otel

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
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

func TestStopCollectorFromPIDStopsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pid stop test skipped on windows")
	}
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	pidPath := filepath.Join(t.TempDir(), "collector.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("write pid failed: %v", err)
	}

	stopCollectorFromPID(pidPath, nil)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("expected process to exit")
	}

	if _, err := os.Stat(pidPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected pid file removed")
	}
}

func TestCollectorClearsActiveOnExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit test skipped on windows")
	}
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}
	collector := &Collector{
		cmd:     cmd,
		done:    make(chan error, 1),
		pidPath: filepath.Join(t.TempDir(), "collector.pid"),
	}
	SetActiveCollector(CollectorInfo{ConfigPath: "test"})

	collector.waitForExit(cmd, collector.done)

	if _, ok := ActiveCollector(); ok {
		t.Fatalf("expected active collector cleared")
	}
}
