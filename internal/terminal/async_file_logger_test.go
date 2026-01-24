package terminal

import (
	"os"
	"testing"
	"time"
)

func TestAsyncFileLoggerWritesAndFlushesOnClose(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/logger.txt"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	logger := newAsyncFileLogger(path, file, time.Hour, 128, 4, func(value string) ([]byte, error) {
		return []byte(value + "\n"), nil
	})

	logger.Write("alpha")
	logger.Write("beta")

	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "alpha\nbeta\n" {
		t.Fatalf("unexpected contents: %q", string(data))
	}
	if logger.Dropped() != 0 {
		t.Fatalf("expected no drops, got %d", logger.Dropped())
	}
}

func TestAsyncFileLoggerDropsOldestWhenBufferFull(t *testing.T) {
	logger := &asyncFileLogger[string]{
		writeCh: make(chan string, 1),
	}

	logger.Write("first")
	logger.Write("second")
	logger.Write("third")

	if logger.Dropped() != 2 {
		t.Fatalf("expected 2 drops, got %d", logger.Dropped())
	}

	got := <-logger.writeCh
	if got != "third" {
		t.Fatalf("expected newest entry, got %q", got)
	}
}
