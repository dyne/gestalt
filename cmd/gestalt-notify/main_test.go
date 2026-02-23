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

func TestRunWithSenderNonZeroWritesStderr(t *testing.T) {
	t.Run("usage error", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := runWithSender([]string{`{"type":"plan-L1-wip"}`}, &stdout, &stderr, nil)
		if code != exitCodeUsage {
			t.Fatalf("expected code %d, got %d", exitCodeUsage, code)
		}
		if strings.TrimSpace(stderr.String()) == "" {
			t.Fatalf("expected stderr output")
		}
	})

	cases := []struct {
		name    string
		code    int
		message string
	}{
		{name: "rejected", code: exitCodeRejected, message: "request rejected"},
		{name: "network", code: exitCodeNetwork, message: "network failure"},
		{name: "session not found", code: exitCodeSessionNotFound, message: "session missing"},
		{name: "invalid payload", code: exitCodeInvalidPayload, message: "invalid payload"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := runWithSender(
				[]string{"--session-id", "term-1", `{"type":"plan-L1-wip","plan_file":"plan.org"}`},
				&stdout,
				&stderr,
				func(Config) error {
					return notifyErr(tc.code, tc.message)
				},
			)
			if code != tc.code {
				t.Fatalf("expected code %d, got %d", tc.code, code)
			}
			if !strings.Contains(stderr.String(), tc.message) {
				t.Fatalf("expected stderr to contain %q, got %q", tc.message, stderr.String())
			}
		})
	}
}

func TestRunWithSenderNormalizesSessionID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithSender(
		[]string{"--session-id", "Coder", `{"type":"plan-L1-wip","plan_file":"plan.org"}`},
		&stdout,
		&stderr,
		func(cfg Config) error {
			if cfg.SessionID != "Coder 1" {
				t.Fatalf("expected normalized session id Coder 1, got %q", cfg.SessionID)
			}
			return notifyErr(exitCodeSessionNotFound, "session not found")
		},
	)
	if code != exitCodeSessionNotFound {
		t.Fatalf("expected code %d, got %d", exitCodeSessionNotFound, code)
	}
	if !strings.Contains(stderr.String(), "session not found") {
		t.Fatalf("expected not found error, got %q", stderr.String())
	}
}
