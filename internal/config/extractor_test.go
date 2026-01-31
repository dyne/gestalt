package config

import (
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gestalt"
)

func TestExtractorExtractsNewFiles(t *testing.T) {
	destDir := t.TempDir()
	expectedHash := embeddedHash(t, "config/agents/coder.toml")
	manifest := map[string]string{
		"agents/coder.toml": expectedHash,
	}

	extractor := Extractor{BackupLimit: 1}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	destPath := filepath.Join(destDir, "agents", "coder.toml")
	actual, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	expected := embeddedFile(t, "config/agents/coder.toml")
	if string(actual) != string(expected) {
		t.Fatalf("extracted contents mismatch")
	}
}

func TestExtractorBuildsManifestWhenEmpty(t *testing.T) {
	destDir := t.TempDir()
	sourceFS := fstest.MapFS{
		"config/agents/example.toml": &fstest.MapFile{Data: []byte("name = \"Example\""), Mode: 0o644},
	}

	extractor := Extractor{BackupLimit: 1}
	if err := extractor.Extract(sourceFS, destDir, nil); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	destPath := filepath.Join(destDir, "agents", "example.toml")
	actual, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(actual) != "name = \"Example\"" {
		t.Fatalf("expected extracted contents to match source")
	}
}

func TestExtractorSkipsMatchingFiles(t *testing.T) {
	destDir := t.TempDir()
	expectedHash := embeddedHash(t, "config/agents/coder.toml")
	manifest := map[string]string{
		"agents/coder.toml": expectedHash,
	}

	extractor := Extractor{BackupLimit: 1}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	destPath := filepath.Join(destDir, "agents", "coder.toml")
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
	destPath := filepath.Join(destDir, "agents", "coder.toml")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	manifest := map[string]string{
		"agents/coder.toml": "",
	}

	extractor := Extractor{BackupLimit: 1}
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
	expected := embeddedFile(t, "config/agents/coder.toml")
	if string(extracted) != string(expected) {
		t.Fatalf("expected extracted contents to match embedded file")
	}
}

func TestExtractorKeepsModifiedFilesWhenNonInteractive(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "agents", "example.toml")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}

	sourceFS := fstest.MapFS{
		"config/agents/example.toml": &fstest.MapFile{Data: []byte("name = \"Example\""), Mode: 0o644},
	}
	expectedHash, err := hashFileFromFS(sourceFS, "config/agents/example.toml")
	if err != nil {
		t.Fatalf("hash source file: %v", err)
	}
	manifest := map[string]string{
		"agents/example.toml": expectedHash,
	}

	reader := &panicReader{}
	extractor := Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: false,
			In:          reader,
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	contents, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest file: %v", err)
	}
	if string(contents) != "custom" {
		t.Fatalf("expected dest to remain unchanged")
	}
	dist, err := os.ReadFile(destPath + ".dist")
	if err != nil {
		t.Fatalf("read dist file: %v", err)
	}
	if string(dist) != "name = \"Example\"" {
		t.Fatalf("expected dist contents to match packaged file")
	}
	if _, err := os.Stat(destPath + ".bck"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file presence: %v", err)
	}
	if reader.called {
		t.Fatalf("expected no reads from stdin in non-interactive mode")
	}
}

type panicReader struct {
	called bool
}

func (p *panicReader) Read(_ []byte) (int, error) {
	p.called = true
	return 0, fmt.Errorf("unexpected read")
}

func TestExtractorReplacesExistingBackup(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "agents", "coder.toml")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("current"), 0o644); err != nil {
		t.Fatalf("write current file: %v", err)
	}
	backupPath := destPath + ".bck"
	if err := os.WriteFile(backupPath, []byte("old-backup"), 0o644); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	manifest := map[string]string{
		"agents/coder.toml": "",
	}

	extractor := Extractor{BackupLimit: 1}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "current" {
		t.Fatalf("expected backup to be replaced with current contents")
	}
}

func TestExtractorBackupLimit(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "agents", "coder.toml")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("current"), 0o644); err != nil {
		t.Fatalf("write current file: %v", err)
	}
	oldBackupOne := destPath + ".bck.20200101-000000-000000000"
	oldBackupTwo := destPath + ".bck.20200102-000000-000000000"
	if err := os.WriteFile(oldBackupOne, []byte("old1"), 0o644); err != nil {
		t.Fatalf("write old backup 1: %v", err)
	}
	if err := os.WriteFile(oldBackupTwo, []byte("old2"), 0o644); err != nil {
		t.Fatalf("write old backup 2: %v", err)
	}
	oldTime := time.Date(1999, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldBackupOne, oldTime, oldTime); err != nil {
		t.Fatalf("set old backup 1 time: %v", err)
	}
	if err := os.Chtimes(oldBackupTwo, oldTime, oldTime); err != nil {
		t.Fatalf("set old backup 2 time: %v", err)
	}

	manifest := map[string]string{
		"agents/coder.toml": "",
	}

	extractor := Extractor{BackupLimit: 2}
	if err := extractor.Extract(gestalt.EmbeddedConfigFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	backups, err := os.ReadDir(filepath.Dir(destPath))
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	var backupCount int
	for _, entry := range backups {
		if strings.HasPrefix(entry.Name(), filepath.Base(destPath)+".bck") {
			backupCount++
		}
	}
	if backupCount > 2 {
		t.Fatalf("expected at most 2 backups, got %d", backupCount)
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
	hasher := fnv.New64a()
	_, _ = hasher.Write(data)
	return fmt.Sprintf("%016x", hasher.Sum64())
}
