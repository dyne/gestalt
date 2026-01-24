//go:build !noscip

package scip

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
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

	version := "v0.0.0"
	assetName := fmt.Sprintf("scip-go_%s_%s_%s.tar.gz", normalizedVersion(version), runtime.GOOS, runtime.GOARCH)
	archivePath := filepath.Join(tempDir, assetName)
	writeTarGz(t, archivePath, "scip-go", []byte("binary"))
	original := builtInIndexers
	builtInIndexers = []Indexer{
		{
			Language: "go",
			Name:     "scip-go",
			Version:  version,
			Binary:   "scip-go",
			URL:      "file://" + archivePath,
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

func TestDownloadIndexerMissingFile(t *testing.T) {
	tempDir := t.TempDir()
	indexerDirOverride = tempDir
	t.Cleanup(func() {
		indexerDirOverride = ""
	})

	version := "v0.0.0"
	assetName := fmt.Sprintf("scip-go_%s_%s_%s.tar.gz", normalizedVersion(version), runtime.GOOS, runtime.GOARCH)
	archivePath := filepath.Join(tempDir, assetName)

	original := builtInIndexers
	builtInIndexers = []Indexer{
		{
			Language: "go",
			Name:     "scip-go",
			Version:  version,
			Binary:   "scip-go",
			URL:      "file://" + archivePath,
		},
	}
	t.Cleanup(func() {
		builtInIndexers = original
	})

	if err := DownloadIndexer("go"); err == nil {
		t.Fatalf("expected missing file error")
	}
}

func TestValidateAssetVersion(t *testing.T) {
	indexer := Indexer{Name: "scip-go", Version: "v1.2.3"}
	url := "https://example.com/scip-go_0.1.0_linux_amd64.tar.gz"
	if err := validateAssetVersion(indexer, url); err == nil {
		t.Fatalf("expected version mismatch error")
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

func writeTarGz(t *testing.T, archivePath, name string, content []byte) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	header := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
}
