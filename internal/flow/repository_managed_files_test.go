package flow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveStaleManagedFlowFilesOnlyDeletesManagedTopLevelFiles(t *testing.T) {
	dir := t.TempDir()

	keepManaged := filepath.Join(dir, "keep.flow.yaml")
	staleManaged := filepath.Join(dir, "stale.flow.yaml")
	otherYAML := filepath.Join(dir, "other.yaml")
	readme := filepath.Join(dir, "README.md")
	hidden := filepath.Join(dir, ".backup.flow.yaml.bak")
	nestedDir := filepath.Join(dir, "nested")
	nestedManaged := filepath.Join(nestedDir, "nested.flow.yaml")

	if err := os.WriteFile(keepManaged, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep managed: %v", err)
	}
	if err := os.WriteFile(staleManaged, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale managed: %v", err)
	}
	if err := os.WriteFile(otherYAML, []byte("other"), 0o644); err != nil {
		t.Fatalf("write other yaml: %v", err)
	}
	if err := os.WriteFile(readme, []byte("readme"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(hidden, []byte("hidden"), 0o644); err != nil {
		t.Fatalf("write hidden file: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(nestedManaged, []byte("nested"), 0o644); err != nil {
		t.Fatalf("write nested managed: %v", err)
	}

	desired := map[string]struct{}{
		"keep.flow.yaml": {},
	}
	if err := removeStaleManagedFlowFiles(dir, desired); err != nil {
		t.Fatalf("remove stale managed files: %v", err)
	}

	if _, err := os.Stat(keepManaged); err != nil {
		t.Fatalf("expected keep managed file to remain: %v", err)
	}
	if _, err := os.Stat(staleManaged); !os.IsNotExist(err) {
		t.Fatalf("expected stale managed file deleted, got %v", err)
	}
	if _, err := os.Stat(otherYAML); err != nil {
		t.Fatalf("expected non-managed yaml to remain: %v", err)
	}
	if _, err := os.Stat(readme); err != nil {
		t.Fatalf("expected readme to remain: %v", err)
	}
	if _, err := os.Stat(hidden); err != nil {
		t.Fatalf("expected hidden backup to remain: %v", err)
	}
	if _, err := os.Stat(nestedManaged); err != nil {
		t.Fatalf("expected nested managed file to remain: %v", err)
	}
}
