package terminal

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func collectWrites(filter OutputFilter, chunks ...[]byte) []byte {
	var out []byte
	for _, chunk := range chunks {
		out = append(out, filter.Write(chunk)...)
	}
	return out
}

func TestUTF8GuardFilterTwoByteRuneSplit(t *testing.T) {
	t.Parallel()

	filter := NewUTF8GuardFilter()
	runeBytes := []byte("é")
	out := collectWrites(filter, runeBytes[:1], runeBytes[1:])
	if !bytes.Equal(out, runeBytes) {
		t.Fatalf("expected rune bytes %v, got %v", runeBytes, out)
	}
}

func TestUTF8GuardFilterThreeByteRuneSplit(t *testing.T) {
	t.Parallel()

	filter := NewUTF8GuardFilter()
	runeBytes := []byte("€")
	out := collectWrites(filter, runeBytes[:2], runeBytes[2:])
	if !bytes.Equal(out, runeBytes) {
		t.Fatalf("expected rune bytes %v, got %v", runeBytes, out)
	}
}

func TestUTF8GuardFilterFourByteRuneSplit(t *testing.T) {
	t.Parallel()

	filter := NewUTF8GuardFilter()
	runeBytes := []byte("😀")
	out := collectWrites(filter, runeBytes[:3], runeBytes[3:])
	if !bytes.Equal(out, runeBytes) {
		t.Fatalf("expected rune bytes %v, got %v", runeBytes, out)
	}
}

func TestUTF8GuardFilterFlushesReplacementRune(t *testing.T) {
	t.Parallel()

	filter := NewUTF8GuardFilter()
	runeBytes := []byte("😀")
	out := collectWrites(filter, runeBytes[:2])
	if len(out) != 0 {
		t.Fatalf("expected no output yet, got %v", out)
	}
	flushed := filter.Flush()
	if !bytes.Equal(flushed, []byte(string(utf8.RuneError))) {
		t.Fatalf("expected replacement rune, got %v", flushed)
	}
}
