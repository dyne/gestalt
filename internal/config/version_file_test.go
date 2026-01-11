package config

import (
	"errors"
	"path/filepath"
	"testing"

	"gestalt/internal/version"
)

func TestVersionFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version.json")
	info := version.VersionInfo{
		Version:   "1.2.3",
		Major:     1,
		Minor:     2,
		Patch:     3,
		Built:     "2026-01-11T12:34:56Z",
		GitCommit: "abc123",
	}

	if err := WriteVersionFile(path, info); err != nil {
		t.Fatalf("write version file: %v", err)
	}
	loaded, err := LoadVersionFile(path)
	if err != nil {
		t.Fatalf("load version file: %v", err)
	}
	if loaded.Version != info.Version || loaded.Major != info.Major || loaded.Minor != info.Minor || loaded.Patch != info.Patch {
		t.Fatalf("loaded version mismatch: %+v", loaded)
	}
	if loaded.Built != info.Built || loaded.GitCommit != info.GitCommit {
		t.Fatalf("loaded metadata mismatch: %+v", loaded)
	}
}

func TestLoadVersionFileMissing(t *testing.T) {
	_, err := LoadVersionFile(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, ErrVersionFileMissing) {
		t.Fatalf("expected ErrVersionFileMissing, got %v", err)
	}
}
