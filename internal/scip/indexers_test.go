package scip

import (
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

func TestDownloadBinaryPrefersCurl(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("curl/wget tests skipped on windows")
	}
	tempDir := t.TempDir()
	writeDownloaderScript(t, filepath.Join(tempDir, "curl"), "-o", "curl")
	writeDownloaderScript(t, filepath.Join(tempDir, "wget"), "-O", "wget")
	setTempPath(t, tempDir)

	destination := filepath.Join(tempDir, "indexer")
	if err := downloadBinary("https://example.invalid/asset", destination); err != nil {
		t.Fatalf("downloadBinary failed: %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if strings.TrimSpace(string(content)) != "curl" {
		t.Fatalf("expected curl downloader, got %s", strings.TrimSpace(string(content)))
	}
}

func TestDownloadBinaryFallsBackToWget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("curl/wget tests skipped on windows")
	}
	tempDir := t.TempDir()
	writeDownloaderScript(t, filepath.Join(tempDir, "wget"), "-O", "wget")
	setTempPath(t, tempDir)

	destination := filepath.Join(tempDir, "indexer")
	if err := downloadBinary("https://example.invalid/asset", destination); err != nil {
		t.Fatalf("downloadBinary failed: %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if strings.TrimSpace(string(content)) != "wget" {
		t.Fatalf("expected wget downloader, got %s", strings.TrimSpace(string(content)))
	}
}

func TestDownloadBinaryReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("curl/wget tests skipped on windows")
	}
	tempDir := t.TempDir()
	script := "#!/bin/sh\necho \"boom\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(tempDir, "curl"), []byte(script), 0o755); err != nil {
		t.Fatalf("write curl script: %v", err)
	}
	setTempPath(t, tempDir)

	destination := filepath.Join(tempDir, "indexer")
	err := downloadBinary("https://example.invalid/asset", destination)
	if err == nil {
		t.Fatalf("expected download error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error output, got %v", err)
	}
	if _, statErr := os.Stat(destination + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp file cleanup, got %v", statErr)
	}
}

func writeDownloaderScript(t *testing.T, path, flag, marker string) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\nout=\"\"\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"%s\" ]; then\n    out=\"$2\"\n    shift 2\n    continue\n  fi\n  shift\ndone\nif [ -z \"$out\" ]; then\n  echo \"missing output\" >&2\n  exit 2\nfi\necho \"%s\" > \"$out\"\n", flag, marker)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write downloader script: %v", err)
	}
}

func setTempPath(t *testing.T, path string) {
	t.Helper()
	original := os.Getenv("PATH")
	if err := os.Setenv("PATH", path); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", original)
	})
}
