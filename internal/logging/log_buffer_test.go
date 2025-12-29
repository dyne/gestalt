package logging

import (
	"sync"
	"testing"
	"time"
)

func TestLogBufferCircular(t *testing.T) {
	buffer := NewLogBuffer(2)
	buffer.Add(LogEntry{Message: "first"})
	buffer.Add(LogEntry{Message: "second"})
	buffer.Add(LogEntry{Message: "third"})

	entries := buffer.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "second" {
		t.Fatalf("expected second, got %q", entries[0].Message)
	}
	if entries[1].Message != "third" {
		t.Fatalf("expected third, got %q", entries[1].Message)
	}
}

func TestLogBufferEntryLimit(t *testing.T) {
	buffer := NewLogBuffer(3)
	buffer.Add(LogEntry{Message: "one"})
	buffer.Add(LogEntry{Message: "two"})

	entries := buffer.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "one" {
		t.Fatalf("expected one, got %q", entries[0].Message)
	}
	if entries[1].Message != "two" {
		t.Fatalf("expected two, got %q", entries[1].Message)
	}
}

func TestLogBufferConcurrentAdds(t *testing.T) {
	buffer := NewLogBuffer(50)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				buffer.Add(LogEntry{
					Timestamp: time.Now(),
					Message:   "entry",
				})
			}
		}(i)
	}
	wg.Wait()

	entries := buffer.List()
	if len(entries) != 50 {
		t.Fatalf("expected 50 entries, got %d", len(entries))
	}
}
