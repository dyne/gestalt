package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGitDirWithDirectory(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	got := ResolveGitDir(workDir)
	if got != gitDir {
		t.Fatalf("expected %q, got %q", gitDir, got)
	}
}

func TestResolveGitDirWithGitFile(t *testing.T) {
	workDir := t.TempDir()
	actual := filepath.Join(workDir, "gitdir")
	if err := os.MkdirAll(actual, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	gitFile := filepath.Join(workDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: gitdir\n"), 0o644); err != nil {
		t.Fatalf("write git file: %v", err)
	}

	got := ResolveGitDir(workDir)
	if got != actual {
		t.Fatalf("expected %q, got %q", actual, got)
	}
}

func TestReadGitOrigin(t *testing.T) {
	workDir := t.TempDir()
	configPath := filepath.Join(workDir, "config")
	contents := []byte("[core]\n\tbare = false\n\n[remote \"origin\"]\n\turl = git@github.com:example/repo.git\n")
	if err := os.WriteFile(configPath, contents, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origin := ReadGitOrigin(configPath)
	if origin != "git@github.com:example/repo.git" {
		t.Fatalf("expected origin, got %q", origin)
	}
}

func TestReadGitBranch(t *testing.T) {
	workDir := t.TempDir()
	headPath := filepath.Join(workDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}

	branch := ReadGitBranch(headPath)
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

func TestReadGitBranchDetached(t *testing.T) {
	workDir := t.TempDir()
	headPath := filepath.Join(workDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("0123456789abcdef\n"), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}

	branch := ReadGitBranch(headPath)
	if branch != "detached@0123456789ab" {
		t.Fatalf("expected detached head, got %q", branch)
	}
}
