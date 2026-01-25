package terminal

import (
	"os/exec"
	"strings"
	"testing"

	"gestalt/internal/logging"
)

func TestSessionFactoryFiltersStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf '\\033[31mfail\\033[0m-----' 1>&2; exit 1")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelDebug, nil)
	factory := NewSessionFactory(SessionFactoryOptions{Logger: logger})

	factory.logShellStartError(sessionCreateRequest{}, "terminal-1", "/bin/sh", "sh", []string{"-c", "exit"}, err)

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatal("expected log entry")
	}
	entry := entries[len(entries)-1]
	stderr := entry.Context["stderr"]
	if strings.Contains(stderr, "\x1b") {
		t.Fatalf("expected stderr filtered, got %q", stderr)
	}
	if strings.Contains(stderr, "-----") {
		t.Fatalf("expected repeated chars collapsed, got %q", stderr)
	}
	if !strings.Contains(stderr, "fail") {
		t.Fatalf("expected stderr content preserved, got %q", stderr)
	}
}
