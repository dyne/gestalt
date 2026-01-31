package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestExtractorDecisionTable(t *testing.T) {
	relPath := "agents/example.toml"
	cases := []struct {
		name            string
		destContent     string
		sourceContent   string
		baselineContent string
		hasBaseline     bool
		interactive     bool
		input           string
		expectDest      string
		expectDist      bool
		expectBackup    bool
	}{
		{
			name:          "baseline missing prompts and keeps",
			destContent:   "custom",
			sourceContent: "new",
			hasBaseline:   false,
			interactive:   false,
			expectDest:    "custom",
			expectDist:    true,
			expectBackup:  false,
		},
		{
			name:            "package updated installs when unmodified",
			destContent:     "old",
			sourceContent:   "new",
			baselineContent: "old",
			hasBaseline:     true,
			interactive:     false,
			expectDest:      "new",
			expectDist:      false,
			expectBackup:    true,
		},
		{
			name:            "package updated prompts when modified",
			destContent:     "custom",
			sourceContent:   "new",
			baselineContent: "old",
			hasBaseline:     true,
			interactive:     false,
			expectDest:      "custom",
			expectDist:      true,
			expectBackup:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			destDir := t.TempDir()
			destPath := writeDestFile(t, destDir, relPath, tc.destContent)

			sourcePath := "config/" + relPath
			sourceFS := fstest.MapFS{
				sourcePath: &fstest.MapFile{Data: []byte(tc.sourceContent), Mode: 0o644},
			}
			newHash := hashFromContent(t, relPath, tc.sourceContent)
			if tc.hasBaseline {
				oldHash := hashFromContent(t, relPath, tc.baselineContent)
				if err := WriteBaselineManifest(destDir, map[string]string{relPath: oldHash}); err != nil {
					t.Fatalf("write baseline: %v", err)
				}
			}

			extractor := Extractor{
				BackupLimit: 1,
				Resolver: &ConffileResolver{
					Interactive: tc.interactive,
					In:          strings.NewReader(tc.input),
					Out:         io.Discard,
				},
			}
			if err := extractor.Extract(sourceFS, destDir, map[string]string{relPath: newHash}); err != nil {
				t.Fatalf("extract failed: %v", err)
			}

			payload, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("read dest file: %v", err)
			}
			if string(payload) != tc.expectDest {
				t.Fatalf("expected dest %q, got %q", tc.expectDest, string(payload))
			}

			distPath := destPath + ".dist"
			if tc.expectDist {
				dist, err := os.ReadFile(distPath)
				if err != nil {
					t.Fatalf("read dist file: %v", err)
				}
				if string(dist) != tc.sourceContent {
					t.Fatalf("expected dist %q, got %q", tc.sourceContent, string(dist))
				}
			} else if _, err := os.Stat(distPath); !os.IsNotExist(err) {
				t.Fatalf("unexpected dist file presence: %v", err)
			}

			backupPath := destPath + ".bck"
			if tc.expectBackup {
				backup, err := os.ReadFile(backupPath)
				if err != nil {
					t.Fatalf("read backup file: %v", err)
				}
				if string(backup) != tc.destContent {
					t.Fatalf("expected backup %q, got %q", tc.destContent, string(backup))
				}
			} else if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
				t.Fatalf("unexpected backup file presence: %v", err)
			}
		})
	}
}

func TestExtractorApplyAllInstalls(t *testing.T) {
	destDir := t.TempDir()
	files := map[string]struct {
		baseline string
		dest     string
		source   string
	}{
		"agents/alpha.toml": {baseline: "alpha-old", dest: "alpha-custom", source: "alpha-new"},
		"agents/bravo.toml": {baseline: "bravo-old", dest: "bravo-custom", source: "bravo-new"},
	}

	sourceFS := fstest.MapFS{}
	manifest := make(map[string]string)
	baseline := make(map[string]string)
	for relPath, data := range files {
		writeDestFile(t, destDir, relPath, data.dest)
		sourcePath := "config/" + relPath
		sourceFS[sourcePath] = &fstest.MapFile{Data: []byte(data.source), Mode: 0o644}
		manifest[relPath] = hashFromContent(t, relPath, data.source)
		baseline[relPath] = hashFromContent(t, relPath, data.baseline)
	}
	if err := WriteBaselineManifest(destDir, baseline); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	output := &bytes.Buffer{}
	extractor := Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: true,
			In:          strings.NewReader("a\n"),
			Out:         output,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if strings.Count(output.String(), "Configuration file 'config/") != 1 {
		t.Fatalf("expected a single prompt after apply-all")
	}

	for relPath, data := range files {
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		payload, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("read dest file: %v", err)
		}
		if string(payload) != data.source {
			t.Fatalf("expected dest %q, got %q", data.source, string(payload))
		}
		backup, err := os.ReadFile(destPath + ".bck")
		if err != nil {
			t.Fatalf("read backup file: %v", err)
		}
		if string(backup) != data.dest {
			t.Fatalf("expected backup %q, got %q", data.dest, string(backup))
		}
		if _, err := os.Stat(destPath + ".dist"); !os.IsNotExist(err) {
			t.Fatalf("unexpected dist file presence: %v", err)
		}
	}
}

func TestExtractorBaselineUpdateAvoidsReprompt(t *testing.T) {
	destDir := t.TempDir()
	relPath := "agents/example.toml"
	destPath := writeDestFile(t, destDir, relPath, "custom")

	sourceContent := "new"
	sourcePath := "config/" + relPath
	sourceFS := fstest.MapFS{
		sourcePath: &fstest.MapFile{Data: []byte(sourceContent), Mode: 0o644},
	}
	manifest := map[string]string{relPath: hashFromContent(t, relPath, sourceContent)}
	if err := WriteBaselineManifest(destDir, map[string]string{relPath: hashFromContent(t, relPath, "old")}); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	extractor := Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: false,
			In:          strings.NewReader(""),
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	distPath := destPath + ".dist"
	distInfo, err := os.Stat(distPath)
	if err != nil {
		t.Fatalf("stat dist file: %v", err)
	}
	baseline, err := LoadBaselineManifest(destDir)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	if baseline[relPath] != manifest[relPath] {
		t.Fatalf("expected baseline to update to new hash")
	}

	reader := &panicReader{}
	extractor = Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: true,
			In:          reader,
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract second run failed: %v", err)
	}
	if reader.called {
		t.Fatalf("expected no prompt after baseline update")
	}
	distInfoAfter, err := os.Stat(distPath)
	if err != nil {
		t.Fatalf("stat dist file: %v", err)
	}
	if !distInfoAfter.ModTime().Equal(distInfo.ModTime()) {
		t.Fatalf("expected dist sidecar to remain unchanged")
	}
}

func TestExtractorRotatesDistSidecars(t *testing.T) {
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "agents", "example.toml")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom file: %v", err)
	}
	distPath := destPath + ".dist"
	if err := os.WriteFile(distPath, []byte("old-dist"), 0o644); err != nil {
		t.Fatalf("write dist file: %v", err)
	}

	sourceFS := fstest.MapFS{
		"config/agents/example.toml": &fstest.MapFile{Data: []byte("new"), Mode: 0o644},
	}
	expectedHash, err := hashFileFromFS(sourceFS, "config/agents/example.toml")
	if err != nil {
		t.Fatalf("hash source file: %v", err)
	}
	manifest := map[string]string{
		"agents/example.toml": expectedHash,
	}

	extractor := Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: false,
			In:          strings.NewReader(""),
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	dist, err := os.ReadFile(distPath)
	if err != nil {
		t.Fatalf("read dist file: %v", err)
	}
	if string(dist) != "new" {
		t.Fatalf("expected dist contents to match packaged file")
	}

	entries, err := os.ReadDir(filepath.Dir(destPath))
	if err != nil {
		t.Fatalf("read dest dir: %v", err)
	}
	var rotated string
	prefix := filepath.Base(destPath) + ".dist."
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			rotated = filepath.Join(filepath.Dir(destPath), entry.Name())
			break
		}
	}
	if rotated == "" {
		t.Fatalf("expected rotated dist sidecar")
	}
	payload, err := os.ReadFile(rotated)
	if err != nil {
		t.Fatalf("read rotated dist: %v", err)
	}
	if string(payload) != "old-dist" {
		t.Fatalf("expected rotated dist to preserve prior contents")
	}
}

func TestExtractorInteractivePromptsAreSerialized(t *testing.T) {
	destDir := t.TempDir()
	files := map[string]string{
		"agents/alpha.toml": "alpha",
		"agents/bravo.toml": "bravo",
	}
	for relPath, contents := range files {
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(destPath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write dest file: %v", err)
		}
	}

	sourceFS := fstest.MapFS{
		"config/agents/alpha.toml": &fstest.MapFile{Data: []byte("new-alpha"), Mode: 0o644},
		"config/agents/bravo.toml": &fstest.MapFile{Data: []byte("new-bravo"), Mode: 0o644},
	}
	manifest := map[string]string{
		"agents/alpha.toml": embeddedHashFromFS(t, sourceFS, "config/agents/alpha.toml"),
		"agents/bravo.toml": embeddedHashFromFS(t, sourceFS, "config/agents/bravo.toml"),
	}

	reader := &serialReader{reader: strings.NewReader("n\nn\n")}
	extractor := Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: true,
			In:          reader,
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if reader.Concurrent() {
		t.Fatalf("expected prompts to be serialized")
	}
}

type serialReader struct {
	reader     *strings.Reader
	reading    int32
	concurrent int32
}

func (s *serialReader) Read(p []byte) (int, error) {
	if !atomic.CompareAndSwapInt32(&s.reading, 0, 1) {
		atomic.StoreInt32(&s.concurrent, 1)
	}
	time.Sleep(5 * time.Millisecond)
	n, err := s.reader.Read(p)
	atomic.StoreInt32(&s.reading, 0)
	return n, err
}

func (s *serialReader) Concurrent() bool {
	return atomic.LoadInt32(&s.concurrent) == 1
}

func embeddedHashFromFS(t *testing.T, sourceFS fs.FS, path string) string {
	t.Helper()
	hash, err := hashFileFromFS(sourceFS, path)
	if err != nil {
		t.Fatalf("hash source file %s: %v", path, err)
	}
	return hash
}

func hashFromContent(t *testing.T, relPath, content string) string {
	t.Helper()
	sourcePath := "config/" + relPath
	sourceFS := fstest.MapFS{
		sourcePath: &fstest.MapFile{Data: []byte(content), Mode: 0o644},
	}
	hash, err := hashFileFromFS(sourceFS, sourcePath)
	if err != nil {
		t.Fatalf("hash content %s: %v", relPath, err)
	}
	return hash
}

func writeDestFile(t *testing.T, destDir, relPath, contents string) string {
	t.Helper()
	destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write dest file: %v", err)
	}
	return destPath
}

func TestExtractorWritesBaselineManifest(t *testing.T) {
	destDir := t.TempDir()
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

	extractor := Extractor{BackupLimit: 1}
	if err := extractor.Extract(sourceFS, destDir, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	baselinePath := filepath.Join(destDir, baselineManifestName)
	payload, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	var baseline map[string]string
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	if baseline["agents/example.toml"] != expectedHash {
		t.Fatalf("expected baseline hash to match packaged hash")
	}

	updatedFS := fstest.MapFS{
		"config/agents/example.toml": &fstest.MapFile{Data: []byte("name = \"Updated\""), Mode: 0o644},
	}
	updatedHash, err := hashFileFromFS(updatedFS, "config/agents/example.toml")
	if err != nil {
		t.Fatalf("hash updated source file: %v", err)
	}
	updatedManifest := map[string]string{
		"agents/example.toml": updatedHash,
	}

	extractor = Extractor{
		BackupLimit: 1,
		Resolver: &ConffileResolver{
			Interactive: false,
			In:          strings.NewReader(""),
			Out:         io.Discard,
		},
	}
	if err := extractor.Extract(updatedFS, destDir, updatedManifest); err != nil {
		t.Fatalf("extract updated failed: %v", err)
	}

	payload, err = os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline after update: %v", err)
	}
	baseline = nil
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("decode baseline after update: %v", err)
	}
	if baseline["agents/example.toml"] != updatedHash {
		t.Fatalf("expected baseline to update to new packaged hash")
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
