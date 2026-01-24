package terminal

import "testing"

func TestContainsDSRDetectsAcrossChunks(t *testing.T) {
	tail := []byte{0x1b, '['}
	chunk := []byte{'6', 'n'}
	if !containsDSR(tail, chunk) {
		t.Fatalf("expected DSR sequence to be detected across chunks")
	}
}

func TestUpdateDSRTailKeepsLastBytes(t *testing.T) {
	tail := []byte("abc")
	chunk := []byte("d")
	updated := updateDSRTail(tail, chunk)
	if string(updated) != "bcd" {
		t.Fatalf("expected tail to keep last bytes, got %q", string(updated))
	}
}

func TestContainsDSRResponseParsesRowsAndColumns(t *testing.T) {
	data := []byte("prefix\x1b[12;34Rsuffix")
	if !containsDSRResponse(data) {
		t.Fatalf("expected DSR response to be detected")
	}
}
