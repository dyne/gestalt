package terminal

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInputBufferAppendsTrimmedCommands(t *testing.T) {
	buffer := NewInputBuffer(5)
	buffer.Append("  hello  ")
	buffer.Append("\n\t ")

	entries := buffer.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", entries)
	}
	if entries[0].Command != "hello" {
		t.Fatalf("expected trimmed command, got %q", entries[0].Command)
	}
	if entries[0].Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestInputBufferDropsOldCommands(t *testing.T) {
	buffer := NewInputBuffer(2)
	buffer.Append("one")
	buffer.Append("two")
	buffer.Append("three")

	entries := buffer.GetAll()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}
	if entries[0].Command != "two" || entries[1].Command != "three" {
		t.Fatalf("expected last two commands, got %v", entries)
	}
}

func TestInputBufferGetRecent(t *testing.T) {
	buffer := NewInputBuffer(5)
	buffer.Append("one")
	buffer.Append("two")
	buffer.Append("three")

	entries := buffer.GetRecent(2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}
	if entries[0].Command != "two" || entries[1].Command != "three" {
		t.Fatalf("expected recent commands, got %v", entries)
	}
}

func TestInputBufferConcurrentAccessDoesNotBlock(t *testing.T) {
	buffer := NewInputBuffer(10)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buffer.Append(fmt.Sprintf("cmd-%d", i))
		}(i)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for concurrent append")
	}
}
