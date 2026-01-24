//go:build !noscip

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt/internal/scip"

	_ "github.com/mattn/go-sqlite3"
)

type indexCommandDeps struct {
	detectLanguages func(string) ([]string, error)
	ensureIndexer   func(string, string) (string, error)
	runIndexer      func(string, string, string) error
	mergeIndexes    func([]string, string) error
	convertToSQLite func(string, string) error
	openIndex       func(string) (*scip.Index, error)
}

func stubIndexCommandDeps(t *testing.T, deps indexCommandDeps) {
	t.Helper()

	originalDetect := detectLanguages
	originalEnsure := ensureIndexer
	originalRun := runIndexer
	originalMerge := mergeIndexes
	originalConvert := convertToSQLite
	originalOpen := openIndex

	t.Cleanup(func() {
		detectLanguages = originalDetect
		ensureIndexer = originalEnsure
		runIndexer = originalRun
		mergeIndexes = originalMerge
		convertToSQLite = originalConvert
		openIndex = originalOpen
	})

	if deps.detectLanguages != nil {
		detectLanguages = deps.detectLanguages
	} else {
		detectLanguages = originalDetect
	}
	if deps.ensureIndexer != nil {
		ensureIndexer = deps.ensureIndexer
	} else {
		ensureIndexer = originalEnsure
	}
	if deps.runIndexer != nil {
		runIndexer = deps.runIndexer
	} else {
		runIndexer = originalRun
	}
	if deps.mergeIndexes != nil {
		mergeIndexes = deps.mergeIndexes
	} else {
		mergeIndexes = originalMerge
	}
	if deps.convertToSQLite != nil {
		convertToSQLite = deps.convertToSQLite
	} else {
		convertToSQLite = originalConvert
	}
	if deps.openIndex != nil {
		openIndex = deps.openIndex
	} else {
		openIndex = originalOpen
	}
}

func TestIndexCommandGoRepo(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	outputPath := filepath.Join(tempDir, "out", "index.db")
	var ensureCalls []string

	stubIndexCommandDeps(t, indexCommandDeps{
		detectLanguages: scip.DetectLanguages,
		ensureIndexer: func(lang, repoPath string) (string, error) {
			ensureCalls = append(ensureCalls, lang)
			return "/tmp/indexer", nil
		},
		runIndexer: func(lang, dir, output string) error {
			if dir != tempDir {
				return errors.New("unexpected directory")
			}
			return os.WriteFile(output, []byte("scip"), 0o644)
		},
		mergeIndexes: func(inputs []string, output string) error {
			t.Fatalf("mergeIndexes should not be called")
			return nil
		},
		convertToSQLite: func(scipPath, dbPath string) error {
			writeStatsDB(t, dbPath, 1, 2, 3)
			return nil
		},
		openIndex: scip.OpenIndex,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runIndexCommand([]string{"--path", tempDir, "--output", outputPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success, got code %d: %s", code, stderr.String())
	}
	if len(ensureCalls) != 1 || ensureCalls[0] != "go" {
		t.Fatalf("expected go indexer check, got %v", ensureCalls)
	}
	if !strings.Contains(stdout.String(), "Indexing complete!") {
		t.Fatalf("expected completion output, got: %s", stdout.String())
	}
}

func TestIndexCommandMultiLanguage(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	outputPath := filepath.Join(tempDir, "index.db")
	mergedCalled := false

	stubIndexCommandDeps(t, indexCommandDeps{
		detectLanguages: scip.DetectLanguages,
		ensureIndexer: func(lang, repoPath string) (string, error) {
			return "/tmp/indexer", nil
		},
		runIndexer: func(lang, dir, output string) error {
			return os.WriteFile(output, []byte("scip"), 0o644)
		},
		mergeIndexes: func(inputs []string, output string) error {
			mergedCalled = true
			if output != filepath.Join(tempDir, "index.scip") {
				return errors.New("unexpected merge output")
			}
			return os.WriteFile(output, []byte("merged"), 0o644)
		},
		convertToSQLite: func(scipPath, dbPath string) error {
			if scipPath != filepath.Join(tempDir, "index.scip") {
				return errors.New("unexpected scip path")
			}
			writeStatsDB(t, dbPath, 2, 4, 6)
			return nil
		},
		openIndex: scip.OpenIndex,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runIndexCommand([]string{"--path", tempDir, "--output", outputPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success, got code %d: %s", code, stderr.String())
	}
	if !mergedCalled {
		t.Fatalf("expected mergeIndexes to be called")
	}
	if !strings.Contains(stdout.String(), "Detected languages:") {
		t.Fatalf("expected detected languages output, got: %s", stdout.String())
	}
}

func TestIndexCommandUnsupportedLanguage(t *testing.T) {
	stubIndexCommandDeps(t, indexCommandDeps{
		detectLanguages: func(string) ([]string, error) {
			return []string{"ruby"}, nil
		},
		ensureIndexer: func(lang, repoPath string) (string, error) {
			return "", errors.New("unknown indexer language")
		},
		runIndexer: func(string, string, string) error {
			t.Fatalf("runIndexer should not be called")
			return nil
		},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runIndexCommand([]string{"--path", "."}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected failure for unsupported language")
	}
	if !strings.Contains(stderr.String(), "No supported languages detected") {
		t.Fatalf("expected unsupported language message, got: %s", stderr.String())
	}
}

func TestIndexCommandSkipsRecentIndex(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "index.db")
	if err := os.WriteFile(outputPath, []byte("db"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	meta := scip.IndexMetadata{
		CreatedAt:   time.Now().UTC(),
		ProjectRoot: tempDir,
		Languages:   []string{"go"},
		FilesHashed: "hash",
	}
	if err := scip.SaveMetadata(outputPath, meta); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	called := false
	stubIndexCommandDeps(t, indexCommandDeps{
		detectLanguages: func(string) ([]string, error) {
			called = true
			return nil, errors.New("unexpected language detection")
		},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runIndexCommand([]string{"--path", tempDir, "--output", outputPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success, got code %d: %s", code, stderr.String())
	}
	if called {
		t.Fatalf("expected language detection to be skipped")
	}
	if !strings.Contains(stderr.String(), "Index was created") {
		t.Fatalf("expected recent index warning, got: %s", stderr.String())
	}
}

func writeStatsDB(t *testing.T, path string, documents, symbols, mentions int) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE documents (id INTEGER PRIMARY KEY, relative_path TEXT);`,
		`CREATE TABLE global_symbols (id INTEGER PRIMARY KEY, symbol TEXT, display_name TEXT);`,
		`CREATE TABLE mentions (id INTEGER PRIMARY KEY, symbol_id INTEGER);`,
		`CREATE TABLE defn_enclosing_ranges (id INTEGER PRIMARY KEY, document_id INTEGER, symbol_id INTEGER);`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	insertRows(t, db, "documents", documents)
	insertRows(t, db, "global_symbols", symbols)
	insertRows(t, db, "mentions", mentions)
}

func insertRows(t *testing.T, db *sql.DB, table string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := db.Exec("INSERT INTO " + table + " DEFAULT VALUES"); err != nil {
			t.Fatalf("insert %s: %v", table, err)
		}
	}
}
