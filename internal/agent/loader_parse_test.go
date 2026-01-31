package agent

import (
	"strings"
	"testing"
)

func TestParseErrorIncludesPosition(t *testing.T) {
	data := []byte("name = \"Test\"\ncli_type =\n")
	_, err := loadAgentFromBytes("bad.toml", data)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	message := err.Error()
	if !strings.Contains(message, "parse agent file bad.toml") {
		t.Fatalf("expected parse error prefix, got %q", message)
	}
	if !strings.Contains(message, "line 2") {
		t.Fatalf("expected line info in error, got %q", message)
	}
}
