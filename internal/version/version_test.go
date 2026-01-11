package version

import "testing"

func TestGetVersionInfo(t *testing.T) {
	previousVersion := Version
	previousMajor := Major
	previousMinor := Minor
	previousPatch := Patch
	previousBuilt := Built
	previousCommit := GitCommit

	Version = "1.2.3"
	Major = "1"
	Minor = "2"
	Patch = "3"
	Built = "2026-01-11T12:34:56Z"
	GitCommit = "abc123"

	t.Cleanup(func() {
		Version = previousVersion
		Major = previousMajor
		Minor = previousMinor
		Patch = previousPatch
		Built = previousBuilt
		GitCommit = previousCommit
	})

	info := GetVersionInfo()
	if info.Version != "1.2.3" {
		t.Fatalf("expected version to be 1.2.3, got %q", info.Version)
	}
	if info.Major != 1 || info.Minor != 2 || info.Patch != 3 {
		t.Fatalf("expected 1.2.3, got %d.%d.%d", info.Major, info.Minor, info.Patch)
	}
	if info.Built != "2026-01-11T12:34:56Z" {
		t.Fatalf("expected built timestamp to be preserved, got %q", info.Built)
	}
	if info.GitCommit != "abc123" {
		t.Fatalf("expected git commit to be preserved, got %q", info.GitCommit)
	}
}
