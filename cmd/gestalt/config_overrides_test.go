package main

import (
	"reflect"
	"testing"
)

func TestParseConfigOverrides(t *testing.T) {
	entries := []string{
		"session.log-max-bytes=5",
		"session.log_max_bytes=6",
		"temporal.max-output-bytes=4096",
		"session.tui-mode=snapshot",
		"session.enabled=true",
		"session.disabled=FALSE",
	}
	overrides, err := parseConfigOverrides(entries)
	if err != nil {
		t.Fatalf("parse overrides: %v", err)
	}
	if value, ok := overrides["session.log-max-bytes"]; !ok || value != int64(6) {
		t.Fatalf("expected session.log-max-bytes 6, got %v", overrides["session.log-max-bytes"])
	}
	if overrides["temporal.max-output-bytes"] != int64(4096) {
		t.Fatalf("expected temporal.max-output-bytes 4096, got %v", overrides["temporal.max-output-bytes"])
	}
	if overrides["session.tui-mode"] != "snapshot" {
		t.Fatalf("expected session.tui-mode snapshot, got %v", overrides["session.tui-mode"])
	}
	if overrides["session.enabled"] != true {
		t.Fatalf("expected session.enabled true, got %v", overrides["session.enabled"])
	}
	if overrides["session.disabled"] != false {
		t.Fatalf("expected session.disabled false, got %v", overrides["session.disabled"])
	}
}

func TestParseConfigOverridesRejectsInvalid(t *testing.T) {
	cases := [][]string{
		{""},
		{"no-equals"},
		{"=missing"},
	}
	for _, entry := range cases {
		if _, err := parseConfigOverrides(entry); err == nil {
			t.Fatalf("expected error for %v", entry)
		}
	}
}

func TestLoadConfigCollectsOverrides(t *testing.T) {
	cfg, err := loadConfig([]string{
		"-c", "session.log-max-bytes=5",
		"-c", "session.tui-mode=snapshot",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := map[string]any{
		"session.log-max-bytes": int64(5),
		"session.tui-mode":     "snapshot",
	}
	if !reflect.DeepEqual(cfg.ConfigOverrides, want) {
		t.Fatalf("expected overrides %v, got %v", want, cfg.ConfigOverrides)
	}
}
