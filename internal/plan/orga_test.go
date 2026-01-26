package plan

import (
	"path/filepath"
	"testing"
)

func TestParseWithOrga(t *testing.T) {
	path := filepath.Join("testdata", "sample.org")
	doc, err := ParseWithOrga(path)
	if err != nil {
		t.Fatalf("ParseWithOrga returned error: %v", err)
	}
	if doc.Type != "document" {
		t.Fatalf("expected document type, got %q", doc.Type)
	}
	title, _ := doc.Properties["title"].(string)
	if title != "Sample Plan" {
		t.Fatalf("expected title Sample Plan, got %q", title)
	}
}

func TestParseWithOrgaMissingFile(t *testing.T) {
	_, err := ParseWithOrga(filepath.Join("testdata", "missing.org"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
