package terminal

import (
	"os"
	"path/filepath"
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

	data, err := os.ReadFile(logger.Path())
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
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, logger, nil)
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
	case <-logger.logger.done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for session logger")
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	if string(data) != "line one\nline two\n" {
		t.Fatalf("unexpected log contents: %q", string(data))
	}
}

func TestReadLastLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma"), 0o644); err != nil {
		t.Fatalf("write history file: %v", err)
	}

	lines, err := readLastLines(path, 2)
	if err != nil {
		t.Fatalf("read last lines: %v", err)
	}
	if len(lines) != 2 || lines[0] != "beta" || lines[1] != "gamma" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestManagerHistoryLinesUsesLatestFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "7-20240101-000000.txt")
	newPath := filepath.Join(dir, "7-20250101-000000.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old history: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new history: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(oldPath, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("set old mtime: %v", err)
	}
	if err := os.Chtimes(newPath, now, now); err != nil {
		t.Fatalf("set new mtime: %v", err)
	}

	manager := NewManager(ManagerOptions{
		Shell:         "/bin/sh",
		SessionLogDir: dir,
	})
	lines, err := manager.HistoryLines("7", 1)
	if err != nil {
		t.Fatalf("history lines: %v", err)
	}
	if len(lines) != 1 || lines[0] != "new" {
		t.Fatalf("unexpected history lines: %v", lines)
	}
}

func TestSessionLoggerDropsOldestChunk(t *testing.T) {
	logger := &SessionLogger{
		logger: &asyncFileLogger[[]byte]{
			writeCh: make(chan []byte, 1),
		},
	}

	logger.Write([]byte("first"))
	logger.Write([]byte("second"))

	if logger.DroppedChunks() != 1 {
		t.Fatalf("expected 1 dropped chunk, got %d", logger.DroppedChunks())
	}

	got := <-logger.logger.writeCh
	if string(got) != "second" {
		t.Fatalf("expected newest chunk to remain, got %q", string(got))
	}
}
