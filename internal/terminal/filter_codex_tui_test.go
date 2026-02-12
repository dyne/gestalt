package terminal

import (
	"bytes"
	"testing"
)

func TestCodexTUIFilterDropsChrome(t *testing.T) {
	t.Parallel()

	filter := NewCodexTUIFilter()
	out := filter.Write([]byte("OpenAI Codex\nOutput line\ncontext left 50%\n"))
	out = append(out, filter.Flush()...)

	if bytes.Contains(out, []byte("OpenAI Codex")) {
		t.Fatalf("expected chrome removed, got %q", out)
	}
	if bytes.Contains(out, []byte("context left")) {
		t.Fatalf("expected context left removed, got %q", out)
	}
	if !bytes.Contains(out, []byte("Output line\n")) {
		t.Fatalf("expected output preserved, got %q", out)
	}
}

func TestCodexTUIFilterBuffersPartialLines(t *testing.T) {
	t.Parallel()

	filter := NewCodexTUIFilter()
	out := filter.Write([]byte("Output "))
	out = append(out, filter.Write([]byte("line\n"))...)
	if !bytes.Equal(out, []byte("Output line\n")) {
		t.Fatalf("expected buffered output, got %q", out)
	}
}
