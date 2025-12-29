package logging

import (
	"testing"
	"time"
)

func TestLogHubBroadcast(t *testing.T) {
	hub := NewLogHub()
	ch, cancel := hub.Subscribe(1)
	defer cancel()

	entry := LogEntry{Message: "hello"}
	hub.Broadcast(entry)

	select {
	case got := <-ch:
		if got.Message != "hello" {
			t.Fatalf("expected message hello, got %q", got.Message)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for log entry")
	}
}

func TestLogHubClose(t *testing.T) {
	hub := NewLogHub()
	ch, cancel := hub.Subscribe(1)
	cancel()
	hub.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel closed")
		}
	default:
	}
}
