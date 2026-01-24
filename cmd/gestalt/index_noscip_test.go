//go:build noscip

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestIndexCommandDisabled(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runIndexCommand([]string{"--help"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code when scip is disabled")
	}
	if !strings.Contains(stderr.String(), "SCIP support disabled") {
		t.Fatalf("expected disabled message, got %q", stderr.String())
	}
}
