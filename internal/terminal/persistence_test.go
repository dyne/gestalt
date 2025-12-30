package terminal

import (
	"os"
	"testing"
	"time"
)

func TestSessionLoggerWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewSessionLogger(dir, "alpha", time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("new session logger: %v", err)
	}

	logger.Write([]byte("hello\n"))
	logger.Write([]byte("world\n"))

	if err := logger.Close(); err != nil {
		t.Fatalf("close session logger: %v", err)
	}

	data, err := os.ReadFile(logger.path)
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	if string(data) != "hello\nworld\n" {
		t.Fatalf("unexpected log contents: %q", string(data))
	}
}

func TestSessionLoggerPersistsSessionOutput(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewSessionLogger(dir, "1", time.Now())
	if err != nil {
		t.Fatalf("new session logger: %v", err)
	}

	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, logger)
	out, cancel := session.Subscribe()
	defer cancel()

	pty.Emit("line one\n")
	if !receiveChunk(t, out, []byte("line one\n")) {
		t.Fatalf("expected first output chunk")
	}
	pty.Emit("line two\n")
	if !receiveChunk(t, out, []byte("line two\n")) {
		t.Fatalf("expected second output chunk")
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}

	select {
	case <-logger.done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for session logger")
	}

	data, err := os.ReadFile(logger.path)
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	if string(data) != "line one\nline two\n" {
		t.Fatalf("unexpected log contents: %q", string(data))
	}
}
