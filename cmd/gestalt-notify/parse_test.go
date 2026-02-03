package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseArgsMissingSessionID(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseArgs([]string{"--agent-id", "codex", `{"type":"agent-turn-complete"}`}, &stderr)
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
	if cfg.EventType != "agent-turn-complete" {
		t.Fatalf("expected event_type agent-turn-complete, got %q", cfg.EventType)
	}
	if cfg.Source != "codex-notify" {
		t.Fatalf("expected source codex-notify, got %q", cfg.Source)
	}
	if cfg.Raw == "" {
		t.Fatalf("expected raw payload to be set")
	}
	if cfg.OccurredAt == nil || !cfg.OccurredAt.Equal(time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected occurred_at: %v", cfg.OccurredAt)
	}
}

func TestParseArgsManualPayload(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", "--event-type", "plan-L1-wip", "--payload", `{"plan_file":"plan.org"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EventType != "plan-L1-wip" {
		t.Fatalf("expected event_type plan-L1-wip, got %q", cfg.EventType)
	}
	if cfg.Source != "manual" {
		t.Fatalf("expected source manual, got %q", cfg.Source)
	}
	if len(cfg.Payload) == 0 {
		t.Fatalf("expected payload to be set")
	}
	if cfg.Raw != "" {
		t.Fatalf("expected raw payload to be empty")
	}
}

func TestParseArgsEventTypeOverride(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", "--event-type", "override", `{"type":"agent-turn-complete"}`}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EventType != "override" {
		t.Fatalf("expected event_type override, got %q", cfg.EventType)
	}
}

func TestParseArgsUsesEnvDefaults(t *testing.T) {
	t.Setenv("GESTALT_URL", "http://example.com")
	t.Setenv("GESTALT_TOKEN", "secret")
	var stderr bytes.Buffer

	cfg, err := parseArgs([]string{"--session-id", "term-1", "--event-type", "plan-L1-wip"}, &stderr)
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

func TestParseArgsMissingAgentID(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", "--event-type", "plan-L1-wip"}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentID != "" {
		t.Fatalf("expected empty agent id, got %q", cfg.AgentID)
	}
}

func TestParseArgsKeepsAgentID(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseArgs([]string{"--session-id", "term-1", "--agent-id", "codex", "--event-type", "plan-L1-wip"}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentID != "codex" {
		t.Fatalf("expected agent id codex, got %q", cfg.AgentID)
	}
}
