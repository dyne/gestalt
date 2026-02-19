package gitlog

import "testing"

func TestParseLogOutputParsesCommitsWithBinaryAndTruncation(t *testing.T) {
	raw := "" +
		"1111111111111111111111111111111111111111\x002026-02-18T00:00:00Z\x00feat(ui): add dashboard\n" +
		"10\t2\tfrontend/src/views/Dashboard.svelte\n" +
		"-\t-\tassets/logo.png\n" +
		"\n" +
		"2222222222222222222222222222222222222222\x002026-02-17T15:00:00Z\x00chore: rename file\n" +
		"5\t1\tinternal/api/{old.go => new.go}\n"

	commits, err := ParseLogOutput(raw, 1)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	first := commits[0]
	if first.ShortSHA != "111111111111" {
		t.Fatalf("unexpected short sha: %q", first.ShortSHA)
	}
	if first.Stats.FilesChanged != 2 {
		t.Fatalf("expected files changed 2, got %d", first.Stats.FilesChanged)
	}
	if first.Stats.LinesAdded != 10 || first.Stats.LinesDeleted != 2 {
		t.Fatalf("unexpected line stats: +%d -%d", first.Stats.LinesAdded, first.Stats.LinesDeleted)
	}
	if !first.Stats.HasBinary {
		t.Fatalf("expected binary flag")
	}
	if !first.FilesTruncated {
		t.Fatalf("expected files truncated")
	}
	if len(first.Files) != 1 {
		t.Fatalf("expected 1 file in payload due to truncation, got %d", len(first.Files))
	}
	if first.Files[0].Path != "frontend/src/views/Dashboard.svelte" {
		t.Fatalf("unexpected first file path: %q", first.Files[0].Path)
	}
	if first.Files[0].Added == nil || *first.Files[0].Added != 10 {
		t.Fatalf("expected added=10 for first file")
	}

	second := commits[1]
	if second.Stats.FilesChanged != 1 {
		t.Fatalf("expected second files changed 1, got %d", second.Stats.FilesChanged)
	}
	if len(second.Files) != 1 || second.Files[0].Path != "internal/api/{old.go => new.go}" {
		t.Fatalf("unexpected rename-ish path: %#v", second.Files)
	}
}

func TestParseLogOutputParsesBinaryFileDetails(t *testing.T) {
	raw := "" +
		"1111111111111111111111111111111111111111\x002026-02-18T00:00:00Z\x00feat(ui): add dashboard\n" +
		"10\t2\tfrontend/src/views/Dashboard.svelte\n" +
		"-\t-\tassets/logo.png\n" +
		"\n" +
		"2222222222222222222222222222222222222222\x002026-02-17T15:00:00Z\x00chore: rename file\n" +
		"5\t1\tinternal/api/{old.go => new.go}\n"

	commits, err := ParseLogOutput(raw, 10)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	first := commits[0]
	if first.ShortSHA != "111111111111" {
		t.Fatalf("unexpected short sha: %q", first.ShortSHA)
	}
	if first.Stats.FilesChanged != 2 {
		t.Fatalf("expected files changed 2, got %d", first.Stats.FilesChanged)
	}
	if !first.Stats.HasBinary {
		t.Fatalf("expected binary flag on stats")
	}
	if first.FilesTruncated {
		t.Fatalf("did not expect files to be truncated")
	}
	if len(first.Files) != 2 {
		t.Fatalf("expected 2 files in payload, got %d", len(first.Files))
	}

	textFile := first.Files[0]
	if textFile.Path != "frontend/src/views/Dashboard.svelte" {
		t.Fatalf("unexpected first file path: %q", textFile.Path)
	}
	if textFile.Added == nil || *textFile.Added != 10 {
		t.Fatalf("expected added=10 for first file")
	}
	if textFile.Deleted == nil || *textFile.Deleted != 2 {
		t.Fatalf("expected deleted=2 for first file")
	}

	binaryFile := first.Files[1]
	if binaryFile.Path != "assets/logo.png" {
		t.Fatalf("unexpected binary file path: %q", binaryFile.Path)
	}
	if !binaryFile.Binary {
		t.Fatalf("expected Binary=true for binary file")
	}
	if binaryFile.Added != nil || binaryFile.Deleted != nil {
		t.Fatalf("expected Added/Deleted to be omitted for binary file, got added=%v deleted=%v", binaryFile.Added, binaryFile.Deleted)
	}
}
func TestParseLogOutputEmpty(t *testing.T) {
	commits, err := ParseLogOutput("", 50)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected empty commits, got %d", len(commits))
	}
}

func TestParseLogOutputReturnsErrorOnInvalidNumstat(t *testing.T) {
	raw := "" +
		"1111111111111111111111111111111111111111\x002026-02-18T00:00:00Z\x00feat(ui): add dashboard\n" +
		"not-a-numstat-line\n"

	_, err := ParseLogOutput(raw, 10)
	if err == nil {
		t.Fatalf("expected parse error for invalid numstat line")
	}
}
