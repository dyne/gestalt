package terminal

import (
	"bytes"
	"testing"
	"time"
)

func TestBroadcasterFansOut(t *testing.T) {
	bcast := NewBroadcaster(10)
	first, cancelFirst := bcast.Subscribe()
	second, cancelSecond := bcast.Subscribe()
	defer cancelFirst()
	defer cancelSecond()

	payload := []byte("hello\n")
	bcast.Broadcast(payload)

	if !receiveChunk(t, first, payload) {
		t.Fatalf("expected first subscriber to receive payload")
	}
	if !receiveChunk(t, second, payload) {
		t.Fatalf("expected second subscriber to receive payload")
	}

	lines := bcast.OutputLines()
	if len(lines) == 0 || lines[0] != "hello" {
		t.Fatalf("expected output lines to include hello, got %v", lines)
	}
}

func TestBroadcasterCloseClosesSubscribers(t *testing.T) {
	bcast := NewBroadcaster(5)
	ch, _ := bcast.Subscribe()
	bcast.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for subscriber close")
	}
}

func TestBroadcasterSlowSubscriberDoesNotBlock(t *testing.T) {
	bcast := NewBroadcaster(5)
	ch, cancel := bcast.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 256; i++ {
			bcast.Broadcast([]byte("x"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("broadcast blocked on slow subscriber")
	}

	// Drain to avoid goroutine leaks.
	select {
	case <-ch:
	default:
	}
}

func receiveChunk(t *testing.T, ch <-chan []byte, expected []byte) bool {
	t.Helper()
	select {
	case got := <-ch:
		return bytes.Equal(got, expected)
	case <-time.After(200 * time.Millisecond):
		return false
	}
}
