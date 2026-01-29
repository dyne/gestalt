package otel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// LoadUILogFixture loads the canonical UI log payload fixture.
func LoadUILogFixture(t *testing.T) map[string]any {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, "testdata", "otel", "ui-log.json")
	payload := readJSONFixture(t, path)
	return payload
}

func readJSONFixture(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return payload
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(cwd))
}
