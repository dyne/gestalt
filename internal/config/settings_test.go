package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gestalt"
)

func TestLoadSettingsOverridesWin(t *testing.T) {
	defaultsPayload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/gestalt.toml")
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gestalt.toml")
	if err := os.WriteFile(path, []byte("[session]\nlog-max-bytes = 1\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	overrides := map[string]any{
		"session.log_max_bytes": int64(5),
	}
	settings, err := LoadSettings(path, defaultsPayload, overrides)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Session.LogMaxBytes != 5 {
		t.Fatalf("expected override to win, got %d", settings.Session.LogMaxBytes)
	}
}

func TestLoadSettingsFileOverridesDefaults(t *testing.T) {
	defaultsPayload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/gestalt.toml")
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gestalt.toml")
	if err := os.WriteFile(path, []byte("[temporal]\nmax-output-bytes = 2048\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	settings, err := LoadSettings(path, defaultsPayload, nil)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Temporal.MaxOutputBytes != 2048 {
		t.Fatalf("expected file to override defaults, got %d", settings.Temporal.MaxOutputBytes)
	}
}

func TestLoadSettingsCodexEventLogging(t *testing.T) {
	defaultsPayload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/gestalt.toml")
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gestalt.toml")
	if err := os.WriteFile(path, []byte("[session]\nlog-codex-events = true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	settings, err := LoadSettings(path, defaultsPayload, nil)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if !settings.Session.LogCodexEvents {
		t.Fatalf("expected log-codex-events true, got false")
	}
}

func TestLoadSettingsSessionUIOverrides(t *testing.T) {
	defaultsPayload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/gestalt.toml")
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gestalt.toml")
	payload := `[session]
scrollback-lines = 4242
font-family = "Courier New, monospace"
font-size = "14px"
input-font-family = "Input Mono"
input-font-size = "12px"
`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	settings, err := LoadSettings(path, defaultsPayload, nil)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Session.ScrollbackLines != 4242 {
		t.Fatalf("expected scrollback-lines 4242, got %d", settings.Session.ScrollbackLines)
	}
	if settings.Session.FontFamily != "Courier New, monospace" {
		t.Fatalf("expected font-family override, got %q", settings.Session.FontFamily)
	}
	if settings.Session.FontSize != "14px" {
		t.Fatalf("expected font-size override, got %q", settings.Session.FontSize)
	}
	if settings.Session.InputFontFamily != "Input Mono" {
		t.Fatalf("expected input font-family override, got %q", settings.Session.InputFontFamily)
	}
	if settings.Session.InputFontSize != "12px" {
		t.Fatalf("expected input font-size override, got %q", settings.Session.InputFontSize)
	}
}
