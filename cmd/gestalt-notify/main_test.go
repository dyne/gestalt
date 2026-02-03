package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWithSenderInvalidPayload(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithSender([]string{"--session-id", "term-1", `{"plan_file":"plan.org"}`}, &stdout, &stderr, nil)
	if code != exitCodeInvalidPayload {
		t.Fatalf("expected code %d, got %d", exitCodeInvalidPayload, code)
	}
	if !strings.Contains(stderr.String(), "payload type is required") {
		t.Fatalf("expected payload type error, got %q", stderr.String())
	}
}
