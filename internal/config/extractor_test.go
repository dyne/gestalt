package config

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gestalt"
)

func TestExtractorExtractsNewFiles(t *testing.T) {
	destDir := t.TempDir()
	expectedHash := embeddedHash(t, "config/agents/codex.json")
	manifest := map[string]string{
		"agents/codex.json": expectedHash,
	}

	extractor := Extractor{}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	destPath := filepath.Join(destDir, "agents", "codex.json")
	actual, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	expected := embeddedFile(t, "config/agents/codex.json")
	if string(actual) != string(expected) {
		t.Fatalf("extracted contents mismatch")
	}
}

func TestExtractorSkipsMatchingFiles(t *testing.T) {
	destDir := t.TempDir()
	expectedHash := embeddedHash(t, "config/agents/codex.json")
	manifest := map[string]string{
		"agents/codex.json": expectedHash,
	}

	extractor := Extractor{}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	destPath := filepath.Join(destDir, "agents", "codex.json")
	oldTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(destPath, oldTime, oldTime); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat extracted file: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Fatalf("expected mod time to remain %v, got %v", oldTime, info.ModTime())
	}
	if _, err := os.Stat(destPath + ".bck"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file presence: %v", err)
	}
}

func TestExtractorBacksUpModifiedFiles(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "agents", "codex.json")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	expectedHash := embeddedHash(t, "config/agents/codex.json")
	manifest := map[string]string{
		"agents/codex.json": expectedHash,
	}

	extractor := Extractor{}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	backupPath := destPath + ".bck"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "custom" {
		t.Fatalf("expected backup contents to match custom file")
	}

	extracted, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	expected := embeddedFile(t, "config/agents/codex.json")
	if string(extracted) != string(expected) {
		t.Fatalf("expected extracted contents to match embedded file")
	}
}

func embeddedFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(gestalt.EmbeddedConfigFS, path)
	if err != nil {
		t.Fatalf("read embedded file %s: %v", path, err)
	}
	return data
}

func embeddedHash(t *testing.T, path string) string {
	t.Helper()
	data := embeddedFile(t, path)
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}
