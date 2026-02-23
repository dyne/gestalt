package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseArgsMissingSessionID(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{`{"type":"agent-turn-complete"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "Usage: gestalt-notify") {
		t.Fatalf("expected usage output, got %q", stderr.String())
	}
}

func TestParseArgsCodexPayload(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", `{"type":"agent-turn-complete","occurred_at":"2025-04-01T10:00:00Z"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Raw != "" {
		t.Fatalf("expected raw payload to be empty")
	}
	if cfg.OccurredAt == nil || !cfg.OccurredAt.Equal(time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected occurred_at: %v", cfg.OccurredAt)
	}
}

func TestParseArgsManualPayload(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", `{"type":"plan-L1-wip","plan_file":"plan.org"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Payload) == 0 {
		t.Fatalf("expected payload to be set")
	}
	if cfg.Raw != "" {
		t.Fatalf("expected raw payload to be empty")
	}
}

func TestParseArgsNormalizesSessionID(t *testing.T) {
	var stderr bytes.Buffer

	cfg, err := parseArgs([]string{"--session-id", "Coder", `{"type":"plan-L1-wip","plan_file":"plan.org"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionID != "Coder 1" {
		t.Fatalf("expected normalized session id, got %q", cfg.SessionID)
	}

	cfg, err = parseArgs([]string{"--session-id", "Coder 2", `{"type":"plan-L1-wip","plan_file":"plan.org"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionID != "Coder 2" {
		t.Fatalf("expected explicit session id to be preserved, got %q", cfg.SessionID)
	}
}

func TestParseArgsEventTypeOverride(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1", "--event-type", "override", `{"type":"agent-turn-complete"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -event-type") {
		t.Fatalf("expected unknown flag output, got %q", stderr.String())
	}
}

func TestParseArgsUsesEnvDefaults(t *testing.T) {
	t.Setenv("GESTALT_URL", "http://example.com")
	t.Setenv("GESTALT_TOKEN", "secret")
	var stderr bytes.Buffer

	cfg, err := parseArgs([]string{"--session-id", "term-1", `{"type":"plan-L1-wip"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "http://example.com" {
		t.Fatalf("expected URL to match env, got %q", cfg.URL)
	}
	if cfg.Token != "secret" {
		t.Fatalf("expected token to match env, got %q", cfg.Token)
	}
}

func TestParseArgsRejectsAgentIDFlag(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1", "--agent-id", "codex", `{"type":"plan-L1-wip"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -agent-id") {
		t.Fatalf("expected unknown flag output, got %q", stderr.String())
	}
}

func TestParseArgsRejectsAgentNameFlag(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1", "--agent-name", "Coder 1", `{"type":"plan-L1-wip"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -agent-name") {
		t.Fatalf("expected unknown flag output, got %q", stderr.String())
	}
}

func TestParseArgsRejectsSourceFlag(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1", "--source", "manual", `{"type":"plan-L1-wip"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -source") {
		t.Fatalf("expected unknown flag output, got %q", stderr.String())
	}
}

func TestParseArgsMissingPayload(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1"}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stderr.String(), "Usage: gestalt-notify") {
		t.Fatalf("expected usage output, got %q", stderr.String())
	}
}

func TestParseArgsPayloadFromStdin(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_, _ = writer.WriteString(`{"type":"plan-L1-wip","plan_file":"plan.org"}`)
	_ = writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
	})

	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", "-"}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Payload) == 0 {
		t.Fatalf("expected payload to be set")
	}
}

func TestParseArgsPayloadTypeRequired(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--session-id", "term-1", `{"plan_file":"plan.org"}`}, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "payload type is required") {
		t.Fatalf("expected payload type error, got %v", err)
	}
}
