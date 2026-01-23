package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/config"
	"gestalt/internal/logging"
	"gestalt/internal/version"
)

func TestPrepareConfigExtractsEmbeddedConfig(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)

	paths, err := prepareConfig(cfg, logger)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	agentPath := filepath.Join(cfg.ConfigDir, "agents", "coder.toml")
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("expected extracted agent at %s: %v", agentPath, err)
	}

	installed, err := config.LoadVersionFile(paths.VersionLoc)
	if err != nil {
		t.Fatalf("load version file: %v", err)
	}
	current := version.GetVersionInfo()
	if installed.Version != current.Version {
		t.Fatalf("expected version %q, got %q", current.Version, installed.Version)
	}
	if installed.Major != current.Major || installed.Minor != current.Minor || installed.Patch != current.Patch {
		t.Fatalf("expected version %d.%d.%d, got %d.%d.%d", current.Major, current.Minor, current.Patch, installed.Major, installed.Minor, installed.Patch)
	}
}

func TestPrepareConfigDevModeSkipsExtraction(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	configDir := filepath.Join(root, "config", "agents")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "example.json"), []byte(`{"name":"Example"}`), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("GESTALT_DEV_MODE", "true")
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.DevMode {
		t.Fatalf("expected dev mode enabled")
	}
	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)

	paths, err := prepareConfig(cfg, logger)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, paths.ConfigDir)); err != nil {
		t.Fatalf("expected dev config dir to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gestalt", "config")); !os.IsNotExist(err) {
		t.Fatalf("expected no extracted config dir, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "version.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no version file, got %v", err)
	}
}
