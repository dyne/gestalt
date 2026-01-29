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

	logger := newAsyncFileLogger(path, file, time.Hour, 128, 4, asyncFileLoggerDropOldest, func(value string) ([]byte, error) {
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
		policy:  asyncFileLoggerDropOldest,
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

func TestAsyncFileLoggerBlocksWhenBufferFull(t *testing.T) {
	logger := &asyncFileLogger[string]{
		writeCh: make(chan string, 1),
		closeCh: make(chan struct{}),
		policy:  asyncFileLoggerBlock,
	}

	logger.Write("first")

	started := make(chan struct{})
	done := make(chan struct{})

	go func() {
		close(started)
		logger.Write("second")
		close(done)
	}()

	<-started

	select {
	case <-done:
		t.Fatalf("expected write to block")
	default:
	}

	<-logger.writeCh

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected write to unblock after drain")
	}

	if logger.Dropped() != 0 {
		t.Fatalf("expected no drops, got %d", logger.Dropped())
	}
	if logger.Blocked() != 1 {
		t.Fatalf("expected 1 blocked write, got %d", logger.Blocked())
	}
}
