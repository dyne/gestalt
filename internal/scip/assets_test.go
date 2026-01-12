package scip

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestExtractAssetsSkipsMatchingHash(t *testing.T) {
	payload := []byte("binary")
	hash, err := hashAssetReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("hashReader failed: %v", err)
	}

	fsys := fstest.MapFS{
		"assets/scip/scip-go": &fstest.MapFile{Data: payload, Mode: 0o755},
	}
	manifest := map[string]string{
		"scip-go": hash,
	}

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "scip-go")
	if err := os.WriteFile(destPath, payload, 0o755); err != nil {
		t.Fatalf("write destination: %v", err)
	}

	stats, err := ExtractAssets(fsys, destDir, manifest)
	if err != nil {
		t.Fatalf("ExtractAssets failed: %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("expected 1 skipped file, got %d", stats.Skipped)
	}
	if stats.Extracted != 0 {
		t.Fatalf("expected 0 extracted files, got %d", stats.Extracted)
	}
}

func TestGetIndexerDirUsesGestaltScip(t *testing.T) {
	original := indexerDirOverride
	indexerDirOverride = ""
	t.Cleanup(func() {
		indexerDirOverride = original
	})

	dir, err := getIndexerDir()
	if err != nil {
		t.Fatalf("getIndexerDir failed: %v", err)
	}
	expectedSuffix := filepath.Join(".gestalt", "scip")
	if !strings.HasSuffix(dir, expectedSuffix) {
		t.Fatalf("expected %q suffix, got %q", expectedSuffix, dir)
	}
}
