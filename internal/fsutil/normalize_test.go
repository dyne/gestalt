package fsutil

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestCleanFSPath(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		want      string
		expectErr bool
	}{
		{name: "empty", input: "", want: "."},
		{name: "clean", input: "config/skills", want: "config/skills"},
		{name: "leading slash", input: "/config/skills", want: "config/skills"},
		{name: "invalid", input: "../outside", expectErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := CleanFSPath(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeFSPaths(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "config", "skills")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	marker := filepath.Join(subdir, "marker.txt")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsys, cleaned, err := NormalizeFSPaths(nil, "", subdir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 path, got %d", len(cleaned))
	}
	if _, err := fs.ReadFile(fsys, path.Join(cleaned[0], "marker.txt")); err != nil {
		t.Fatalf("read marker: %v", err)
	}
}

func TestNormalizeFSPathsWithFS(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "config", "skills")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	marker := filepath.Join(subdir, "marker.txt")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsys, cleaned, err := NormalizeFSPaths(os.DirFS(root), "", "config/skills")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 path, got %d", len(cleaned))
	}
	if _, err := fs.ReadFile(fsys, path.Join(cleaned[0], "marker.txt")); err != nil {
		t.Fatalf("read marker: %v", err)
	}
}

func TestReadDirOrEmpty(t *testing.T) {
	root := t.TempDir()
	fsys := os.DirFS(root)
	entries, err := ReadDirOrEmpty(fsys, "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Fatalf("expected nil entries, got %d", len(entries))
	}
}
