package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDesktopConfigDirKeepsCustomPath(t *testing.T) {
	customDir := filepath.Join(t.TempDir(), "config")
	if got := resolveDesktopConfigDir(customDir); got != customDir {
		t.Fatalf("expected %q, got %q", customDir, got)
	}
}

func TestResolveDesktopConfigDirUsesLegacyConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	legacyDir := filepath.Join(homeDir, ".gestalt", "config")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}

	got := resolveDesktopConfigDir(filepath.Join(".gestalt", "config"))
	if got != legacyDir {
		t.Fatalf("expected %q, got %q", legacyDir, got)
	}
}

func TestResolveDesktopConfigDirUsesUserConfigDir(t *testing.T) {
	homeDir := t.TempDir()
	xdgDir := filepath.Join(homeDir, "xdg")
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	got := resolveDesktopConfigDir(filepath.Join(".gestalt", "config"))
	expected := filepath.Join(xdgDir, "gestalt", "config")
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
