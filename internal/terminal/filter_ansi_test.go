package terminal

import (
	"bytes"
	"testing"
)

func collectAnsiWrites(filter OutputFilter, chunks ...[]byte) []byte {
	var out []byte
	for _, chunk := range chunks {
		out = append(out, filter.Write(chunk)...)
	}
	return out
}

func TestANSIStripFilterRemovesCSI(t *testing.T) {
	t.Parallel()

	filter := NewANSIStripFilter()
	out := collectAnsiWrites(filter,
		[]byte("ok\x1b["),
		[]byte("31mred\x1b[0m done"),
	)
	if !bytes.Equal(out, []byte("okred done")) {
		t.Fatalf("expected stripped output, got %q", out)
	}
	if bytes.Contains(out, []byte{0x1b}) {
		t.Fatal("expected no escape bytes in output")
	}
}

func TestANSIStripFilterRemovesOSCAndDCS(t *testing.T) {
	t.Parallel()

	filter := NewANSIStripFilter()
	out := collectAnsiWrites(filter, []byte("before\x1b]0;title\x07middle\x1bPdata\x1b\\after"))
	if !bytes.Equal(out, []byte("beforemiddleafter")) {
		t.Fatalf("expected stripped output, got %q", out)
	}
}

func TestANSIStripFilterPreservesWhitespace(t *testing.T) {
	t.Parallel()

	filter := NewANSIStripFilter()
	out := collectAnsiWrites(filter, []byte("line1\r\n\tline2\bX"))
	if !bytes.Equal(out, []byte("line1\r\n\tline2X")) {
		t.Fatalf("expected preserved whitespace, got %q", out)
	}
}
