//go:build !noscip

package scip

import (
	"os"
	"path/filepath"
	"testing"

	scip "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func TestMergeIndexes(t *testing.T) {
	tempDir := t.TempDir()
	indexPath1 := filepath.Join(tempDir, "one.scip")
	indexPath2 := filepath.Join(tempDir, "two.scip")
	outputPath := filepath.Join(tempDir, "merged.scip")

	index1 := &scip.Index{
		Metadata: &scip.Metadata{ProjectRoot: "."},
		Documents: []*scip.Document{
			{RelativePath: "foo.go", Language: "go"},
		},
		ExternalSymbols: []*scip.SymbolInformation{
			{Symbol: "ext1"},
		},
	}
	index2 := &scip.Index{
		Documents: []*scip.Document{
			{RelativePath: "bar.go", Language: "go"},
		},
		ExternalSymbols: []*scip.SymbolInformation{
			{Symbol: "ext2"},
		},
	}

	writeIndexFile(t, indexPath1, index1)
	writeIndexFile(t, indexPath2, index2)

	if err := MergeIndexes([]string{indexPath1, indexPath2}, outputPath); err != nil {
		t.Fatalf("MergeIndexes failed: %v", err)
	}

	merged := readIndexFile(t, outputPath)
	if len(merged.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(merged.Documents))
	}
	if len(merged.ExternalSymbols) != 2 {
		t.Fatalf("expected 2 external symbols, got %d", len(merged.ExternalSymbols))
	}
}

func TestMergeIndexesDuplicateDocument(t *testing.T) {
	tempDir := t.TempDir()
	indexPath1 := filepath.Join(tempDir, "one.scip")
	indexPath2 := filepath.Join(tempDir, "two.scip")

	index1 := &scip.Index{
		Documents: []*scip.Document{
			{RelativePath: "dup.go", Language: "go"},
		},
	}
	index2 := &scip.Index{
		Documents: []*scip.Document{
			{RelativePath: "dup.go", Language: "go"},
		},
	}

	writeIndexFile(t, indexPath1, index1)
	writeIndexFile(t, indexPath2, index2)

	if err := MergeIndexes([]string{indexPath1, indexPath2}, filepath.Join(tempDir, "merged.scip")); err == nil {
		t.Fatalf("expected error for duplicate document path")
	}
}

func writeIndexFile(t *testing.T, path string, index *scip.Index) {
	t.Helper()

	payload, err := proto.Marshal(index)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

func readIndexFile(t *testing.T, path string) *scip.Index {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var index scip.Index
	if err := proto.Unmarshal(payload, &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	return &index
}
