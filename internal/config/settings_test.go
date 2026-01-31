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
