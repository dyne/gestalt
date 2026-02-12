package terminal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeScrollbackChunks(filter OutputFilter, data []byte, chunkSize int) []byte {
	var out []byte
	for start := 0; start < len(data); start += chunkSize {
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}
		out = append(out, filter.Write(data[start:end])...)
	}
	out = append(out, filter.Flush()...)
	return out
}

func TestScrollbackVTEmitsScrolledLines(t *testing.T) {
	t.Parallel()

	filter := NewScrollbackVTFilter()
	filter.Resize(10, 3)

	out := writeScrollbackChunks(filter, []byte("one\ntwo\nthree\nfour\n"), 8)
	if !bytes.Contains(out, []byte("one\n")) {
		t.Fatalf("expected scrolled line, got %q", out)
	}
	if !bytes.Contains(out, []byte("two\n")) || !bytes.Contains(out, []byte("three\n")) || !bytes.Contains(out, []byte("four\n")) {
		t.Fatalf("expected flushed lines, got %q", out)
	}
}

func TestScrollbackVTEmitsLineOnLineFeed(t *testing.T) {
	t.Parallel()

	filter := NewScrollbackVTFilter()
	filter.Resize(20, 5)

	out := filter.Write([]byte("hello world\n"))
	if !bytes.Contains(out, []byte("hello world\n")) {
		t.Fatalf("expected linefeed emission, got %q", out)
	}
}

func TestScrollbackVTReducesTUISample(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "terminal_filters", "codex_tui_sample.bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	filter := NewScrollbackVTFilter()
	filter.Resize(40, 3)
	out := writeScrollbackChunks(filter, data, 64)

	if bytes.Contains(out, []byte{0x1b}) {
		t.Fatalf("expected no escape bytes, got %q", out)
	}
	if !bytes.Contains(out, []byte("Output line 1")) {
		t.Fatalf("expected output line 1, got %q", out)
	}
	if !bytes.Contains(out, []byte("Output line 4")) {
		t.Fatalf("expected output line 4, got %q", out)
	}
	if bytes.Contains(out, []byte("Status")) {
		t.Fatalf("expected status churn removed, got %q", out)
	}
	if len(out)*20 >= len(data) {
		t.Fatalf("expected output to be < 5%% of input, got %d vs %d", len(out), len(data))
	}
}
