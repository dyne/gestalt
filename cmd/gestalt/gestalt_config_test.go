package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGestaltConfigSelectsPath(t *testing.T) {
	root := t.TempDir()
	devPath := filepath.Join(root, "config", gestaltConfigFilename)
	prodPath := filepath.Join(root, ".gestalt", "config", gestaltConfigFilename)

	if err := os.MkdirAll(filepath.Dir(devPath), 0o755); err != nil {
		t.Fatalf("mkdir dev config dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(prodPath), 0o755); err != nil {
		t.Fatalf("mkdir prod config dir: %v", err)
	}

	if err := os.WriteFile(devPath, []byte("dev"), 0o644); err != nil {
		t.Fatalf("write dev config: %v", err)
	}
	if err := os.WriteFile(prodPath, []byte("prod"), 0o644); err != nil {
		t.Fatalf("write prod config: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	paths := configPaths{ConfigDir: filepath.Dir(prodPath)}

	payload, path, err := loadGestaltConfig(Config{DevMode: false}, paths)
	if err != nil {
		t.Fatalf("load prod config: %v", err)
	}
	if string(payload) != "prod" {
		t.Fatalf("expected prod payload, got %q", payload)
	}
	if path != prodPath {
		t.Fatalf("expected prod path %q, got %q", prodPath, path)
	}

	payload, path, err = loadGestaltConfig(Config{DevMode: true}, paths)
	if err != nil {
		t.Fatalf("load dev config: %v", err)
	}
	if string(payload) != "dev" {
		t.Fatalf("expected dev payload, got %q", payload)
	}
	if path != filepath.Join("config", gestaltConfigFilename) {
		t.Fatalf("expected dev path %q, got %q", filepath.Join("config", gestaltConfigFilename), path)
	}
}
