package scip

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectLanguages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.py"), []byte("print('hi')"), 0o644); err != nil {
		t.Fatalf("write script.py: %v", err)
	}

	languages, err := DetectLanguages(dir)
	if err != nil {
		t.Fatalf("DetectLanguages failed: %v", err)
	}

	expected := []string{"go", "typescript", "python"}
	if strings.Join(languages, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected %v, got %v", expected, languages)
	}
}

func TestDownloadIndexer(t *testing.T) {
	tempDir := t.TempDir()
	indexerDirOverride = tempDir
	t.Cleanup(func() {
		indexerDirOverride = ""
	})

	sourcePath := filepath.Join(tempDir, "source-binary")
	if err := os.WriteFile(sourcePath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("write source binary: %v", err)
	}

	original := builtInIndexers
	builtInIndexers = []Indexer{
		{
			Language: "go",
			Name:     "scip-go",
			Version:  "v0.0.0",
			Binary:   "scip-go",
			URL:      "file://" + sourcePath,
		},
	}
	t.Cleanup(func() {
		builtInIndexers = original
	})

	if err := DownloadIndexer("go"); err != nil {
		t.Fatalf("DownloadIndexer failed: %v", err)
	}

	indexerPath, err := indexerBinaryPath(builtInIndexers[0])
	if err != nil {
		t.Fatalf("indexerBinaryPath: %v", err)
	}

	info, err := os.Stat(indexerPath)
	if err != nil {
		t.Fatalf("indexer not found: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("indexer not executable")
	}
}

func TestRunIndexer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test skipped on windows")
	}
	tempDir := t.TempDir()
	indexerDirOverride = tempDir
	t.Cleanup(func() {
		indexerDirOverride = ""
	})

	indexerPath := filepath.Join(tempDir, "scip-go")
	script := "#!/bin/sh\noutput=\"\"\nprev=\"\"\nfor arg in \"$@\"; do\n  if [ \"$prev\" = \"--output\" ]; then\n    output=\"$arg\"\n  fi\n  prev=\"$arg\"\ndone\nif [ -n \"$output\" ]; then\n  echo \"ok\" > \"$output\"\nfi\n"
	if err := os.WriteFile(indexerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	original := builtInIndexers
	builtInIndexers = []Indexer{
		{
			Language: "go",
			Name:     "scip-go",
			Version:  "v0.0.0",
			Binary:   "scip-go",
			URL:      "http://example.invalid",
		},
	}
	t.Cleanup(func() {
		builtInIndexers = original
	})

	output := filepath.Join(tempDir, "index.scip")
	if err := RunIndexer("go", tempDir, output); err != nil {
		t.Fatalf("RunIndexer failed: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}
