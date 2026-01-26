package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanPlansDirectoryMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	plans, err := ScanPlansDirectory(dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected empty plans list, got %d", len(plans))
	}
}

func TestScanPlansDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plans dir: %v", err)
	}

	alpha := `#+TITLE: Alpha
#+SUBTITLE: First
#+DATE: 2026-01-02
* TODO [#A] Alpha L1
** TODO [#B] Alpha L2
`
	beta := `#+TITLE: Beta
#+SUBTITLE: Second
#+DATE: 2026-01-03
* WIP [#A] Beta L1
** DONE [#C] Beta L2
`
	if err := os.WriteFile(filepath.Join(dir, "2026-01-02-alpha.org"), []byte(alpha), 0o644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-01-03-beta.org"), []byte(beta), 0o644); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	plans, err := ScanPlansDirectory(dir)
	if err != nil {
		t.Fatalf("scan plans: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}
	if plans[0].Filename != "2026-01-03-beta.org" {
		t.Fatalf("expected beta plan first, got %q", plans[0].Filename)
	}
	if plans[0].Metadata.Title != "Beta" {
		t.Fatalf("expected beta title, got %q", plans[0].Metadata.Title)
	}
}

func TestIsValidFilename(t *testing.T) {
	cases := map[string]bool{
		"2026-01-01-plan.org": true,
		"note.txt":            false,
		"../escape.org":       false,
		"nested/plan.org":     false,
		"nested\\plan.org":    false,
	}
	for name, expected := range cases {
		if isValidFilename(name) != expected {
			t.Fatalf("expected %q validity %v", name, expected)
		}
	}
}
