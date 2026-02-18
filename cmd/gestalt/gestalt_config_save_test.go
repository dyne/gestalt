package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gestalt/internal/config/tomlkeys"
)

func TestSaveGestaltConfigAddsMissingKeys(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".gestalt", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(configDir, gestaltConfigFilename)
	if err := os.WriteFile(path, []byte("[session]\nlog-max-bytes = 123\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	saveGestaltConfigDefaults(Config{DevMode: false}, configPaths{ConfigDir: configDir}, nil)

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	store, err := tomlkeys.Decode(payload)
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if value, ok := store.GetInt("session.log-max-bytes"); !ok || value != 123 {
		t.Fatalf("expected session.log-max-bytes 123, got %d", value)
	}
	if value, ok := store.GetInt("session.history-scan-max-bytes"); !ok || value != 2097152 {
		t.Fatalf("expected history-scan-max-bytes 2097152, got %d", value)
	}
}

func TestSaveGestaltConfigIsStable(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".gestalt", "config")
	paths := configPaths{ConfigDir: configDir}
	cfg := Config{DevMode: false}

	saveGestaltConfigDefaults(cfg, paths, nil)
	first, err := os.ReadFile(filepath.Join(configDir, gestaltConfigFilename))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	saveGestaltConfigDefaults(cfg, paths, nil)
	second, err := os.ReadFile(filepath.Join(configDir, gestaltConfigFilename))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("expected config to be stable across saves")
	}
}

func TestSaveGestaltConfigCanonicalizesKeys(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".gestalt", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(configDir, gestaltConfigFilename)
	seed := "session.log_max_bytes = 123\nsession.history_scan_max_bytes = 1024\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	saveGestaltConfigDefaults(Config{DevMode: false}, configPaths{ConfigDir: configDir}, nil)

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	rendered := string(payload)
	if strings.Contains(rendered, "session.log_max_bytes") {
		t.Fatalf("expected dotted keys to be canonicalized")
	}
	if !strings.Contains(rendered, "[session]") {
		t.Fatalf("expected session section")
	}
	if !strings.Contains(rendered, "log-max-bytes = 123") {
		t.Fatalf("expected log-max-bytes to be preserved")
	}
	if !strings.Contains(rendered, "history-scan-max-bytes = 1024") {
		t.Fatalf("expected history-scan-max-bytes to be preserved")
	}

	store, err := tomlkeys.Decode(payload)
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if value, ok := store.GetInt("session.log-max-bytes"); !ok || value != 123 {
		t.Fatalf("expected session.log-max-bytes 123, got %d", value)
	}
	if value, ok := store.GetInt("session.history-scan-max-bytes"); !ok || value != 1024 {
		t.Fatalf("expected session.history-scan-max-bytes 1024, got %d", value)
	}
}
