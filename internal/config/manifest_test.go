package config

import (
	"embed"
	"errors"
	"testing"
)

//go:embed testdata/config/manifest.json
var manifestFS embed.FS

func TestLoadManifest(t *testing.T) {
	manifest, err := loadManifestFromPath(manifestFS, "testdata/config/manifest.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest["agents/example.toml"] != "abc123" {
		t.Fatalf("expected agents/example.toml hash to be abc123, got %q", manifest["agents/example.toml"])
	}
	if manifest["skills/core/SKILL.md"] != "def456" {
		t.Fatalf("expected skills/core/SKILL.md hash to be def456, got %q", manifest["skills/core/SKILL.md"])
	}
}

func TestLoadManifestMissing(t *testing.T) {
	_, err := loadManifestFromPath(manifestFS, "testdata/config/missing.json")
	if !errors.Is(err, ErrManifestMissing) {
		t.Fatalf("expected ErrManifestMissing, got %v", err)
	}
}
