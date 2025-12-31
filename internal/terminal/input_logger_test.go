package terminal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInputLoggerWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	logger, err := NewInputLogger(dir, "agent", createdAt)
	if err != nil {
		t.Fatalf("new input logger: %v", err)
	}

	entry1 := InputEntry{Command: " first ", Timestamp: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	entry2 := InputEntry{Command: "second", Timestamp: time.Date(2025, 1, 2, 3, 4, 6, 0, time.UTC)}
	logger.Write(entry1)
	logger.Write(entry2)

	if err := logger.Close(); err != nil {
		t.Fatalf("close input logger: %v", err)
	}

	wantPath := filepath.Join(dir, "agent-20250102-030405.jsonl")
	if logger.Path() != wantPath {
		t.Fatalf("expected path %q, got %q", wantPath, logger.Path())
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read input log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %v", lines)
	}

	var got []InputEntry
	for _, line := range lines {
		var entry InputEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal entry: %v", err)
		}
		got = append(got, entry)
	}

	if got[0].Command != "first" || !got[0].Timestamp.Equal(entry1.Timestamp) {
		t.Fatalf("unexpected first entry: %+v", got[0])
	}
	if got[1].Command != "second" || !got[1].Timestamp.Equal(entry2.Timestamp) {
		t.Fatalf("unexpected second entry: %+v", got[1])
	}
}
