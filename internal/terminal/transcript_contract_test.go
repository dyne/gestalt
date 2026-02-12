package terminal

import (
	"bytes"
	"errors"
	"testing"
	"unicode/utf8"
)

func validateTranscriptContract(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("terminal output is not valid UTF-8")
	}
	if bytes.IndexByte(data, 0x1b) != -1 {
		return errors.New("terminal output contains escape bytes")
	}
	return nil
}

func TestTranscriptContractAllowsUTF8(t *testing.T) {
	t.Parallel()

	data := []byte("Token usage\t45%\nNext line\n")
	if err := validateTranscriptContract(data); err != nil {
		t.Fatalf("expected valid transcript, got %v", err)
	}
}

func TestTranscriptContractRejectsEscapes(t *testing.T) {
	t.Parallel()

	data := []byte("Status\x1b[2K\rReady\n")
	if err := validateTranscriptContract(data); err == nil {
		t.Fatal("expected escape bytes to be rejected")
	}
}
