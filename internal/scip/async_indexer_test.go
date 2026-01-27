//go:build !noscip

package scip

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	scipproto "github.com/sourcegraph/scip/bindings/go/scip"
)

func TestAsyncIndexerUsesExistingScipWhenIndexerMissing(t *testing.T) {
	projectRoot := t.TempDir()
	scipDir := filepath.Join(projectRoot, ".gestalt", "scip")
	if err := os.MkdirAll(scipDir, 0o755); err != nil {
		t.Fatalf("create scip dir: %v", err)
	}

	indexPath := filepath.Join(scipDir, "index.scip")
	existing := filepath.Join(scipDir, "index-go.scip")
	writeIndexFile(t, existing, &scipproto.Index{
		Metadata: &scipproto.Metadata{ProjectRoot: projectRoot},
		Documents: []*scipproto.Document{
			{RelativePath: "main.go", Language: "go"},
		},
	})

	indexer := NewAsyncIndexer(nil)
	indexer.detectLanguages = func(string) ([]string, error) {
		return []string{"go"}, nil
	}
	indexer.findIndexerPath = func(string, string) (string, error) {
		return "", nil
	}
	if !indexer.StartAsync(IndexRequest{ProjectRoot: projectRoot, ScipDir: scipDir, IndexPath: indexPath}) {
		t.Fatal("expected indexing to start")
	}

	status := waitForIndexComplete(t, indexer)
	if status.Error != "" {
		t.Fatalf("expected no error, got %q", status.Error)
	}
	if status.InProgress {
		t.Fatal("expected indexing to be complete")
	}
	if len(status.Languages) != 1 || status.Languages[0] != "go" {
		t.Fatalf("unexpected languages: %#v", status.Languages)
	}
	if !fileExists(indexPath) {
		t.Fatalf("expected merged scip at %s", indexPath)
	}

}

func TestAsyncIndexerMergesMultipleIndexes(t *testing.T) {
	projectRoot := t.TempDir()
	scipDir := filepath.Join(projectRoot, ".gestalt", "scip")
	if err := os.MkdirAll(scipDir, 0o755); err != nil {
		t.Fatalf("create scip dir: %v", err)
	}

	indexPath := filepath.Join(scipDir, "index.scip")
	mergedPath := filepath.Join(scipDir, mergedScipName)

	indexer := NewAsyncIndexer(nil)
	indexer.detectLanguages = func(string) ([]string, error) {
		return []string{"go", "typescript"}, nil
	}
	indexer.findIndexerPath = func(string, string) (string, error) {
		return "/bin/echo", nil
	}
	indexer.runIndexer = func(language, _ string, output string) error {
		relativePath := language + ".txt"
		writeIndexFile(t, output, &scipproto.Index{
			Documents: []*scipproto.Document{
				{RelativePath: relativePath, Language: language},
			},
		})
		return nil
	}
	mergeCalled := false
	indexer.mergeIndexes = func(inputs []string, output string) error {
		mergeCalled = true
		writeIndexFile(t, output, &scipproto.Index{
			Documents: []*scipproto.Document{
				{RelativePath: "merged.txt", Language: "go"},
			},
		})
		return nil
	}
	indexer.buildMetadata = func(root string, languages []string) (IndexMetadata, error) {
		return IndexMetadata{CreatedAt: time.Now().UTC(), ProjectRoot: root, Languages: languages, FilesHashed: "hash"}, nil
	}
	indexer.saveMetadata = func(string, IndexMetadata) error { return nil }

	if !indexer.StartAsync(IndexRequest{ProjectRoot: projectRoot, ScipDir: scipDir, IndexPath: indexPath}) {
		t.Fatal("expected indexing to start")
	}

	status := waitForIndexComplete(t, indexer)
	if status.Error != "" {
		t.Fatalf("expected no error, got %q", status.Error)
	}
	if !mergeCalled {
		t.Fatal("expected merge to be called")
	}
	if !fileExists(mergedPath) {
		t.Fatalf("expected merged scip at %s", mergedPath)
	}
}

func waitForIndexComplete(t *testing.T, indexer *AsyncIndexer) IndexStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := indexer.Status()
		if !status.InProgress && !status.CompletedAt.IsZero() {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("indexing did not complete: %#v", indexer.Status())
	return IndexStatus{}
}
