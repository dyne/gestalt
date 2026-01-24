package otel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogRecordsHonorsMaxLimit(t *testing.T) {
	t.Setenv("GESTALT_OTEL_MAX_RECORDS", "1")
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")
	lines := []string{
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1","severityText":"INFO","body":{"stringValue":"first"}}]}]}]}`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"2","severityText":"INFO","body":{"stringValue":"second"}}]}]}]}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	records, err := ReadLogRecords(path)
	if err != nil {
		t.Fatalf("ReadLogRecords error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	body := records[0]["body"].(map[string]any)["stringValue"]
	if body != "second" {
		t.Fatalf("expected last record, got %v", body)
	}
}
