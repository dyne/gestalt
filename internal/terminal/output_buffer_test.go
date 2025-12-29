package terminal

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOutputBufferTracksLinesAndCarry(t *testing.T) {
	buffer := NewOutputBuffer(10)

	buffer.Append([]byte("hello"))
	lines := buffer.Lines()
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("expected carry line, got %v", lines)
	}

	buffer.Append([]byte(" world\nnext\npartial"))
	lines = buffer.Lines()
	want := []string{"hello world", "next", "partial"}
	if len(lines) != len(want) {
		t.Fatalf("expected %v, got %v", want, lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, lines)
		}
	}
}

func TestOutputBufferDropsOldLines(t *testing.T) {
	buffer := NewOutputBuffer(2)
	buffer.Append([]byte("one\ntwo\nthree\n"))
	lines := buffer.Lines()
	want := []string{"three", ""}
	if len(lines) != len(want) {
		t.Fatalf("expected %v, got %v", want, lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, lines)
		}
	}
}

func TestOutputBufferIgnoresEmptyAppend(t *testing.T) {
	buffer := NewOutputBuffer(5)
	buffer.Append(nil)
	if lines := buffer.Lines(); len(lines) != 0 {
		t.Fatalf("expected no lines, got %v", lines)
	}
}

func TestOutputBufferConcurrentAccessDoesNotBlock(t *testing.T) {
	buffer := NewOutputBuffer(10)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buffer.Append([]byte(strings.Repeat("x", i%5) + "\n"))
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
