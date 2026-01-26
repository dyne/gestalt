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

func TestReadLogRecordsInjectsResourceAndScope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")
	line := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"gestalt"}}]},"scopeLogs":[{"scope":{"name":"gestalt/test"},"logRecords":[{"timeUnixNano":"1","severityText":"INFO","body":{"stringValue":"hello"}}]}]}]}`
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	records, err := ReadLogRecords(path)
	if err != nil {
		t.Fatalf("ReadLogRecords error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	resource := records[0]["resource"].(map[string]any)
	if resource == nil {
		t.Fatalf("expected resource to be injected")
	}
	attributes := resource["attributes"].([]any)
	if len(attributes) != 1 {
		t.Fatalf("expected 1 resource attribute, got %d", len(attributes))
	}
	scope := records[0]["scope"].(map[string]any)
	if scope == nil {
		t.Fatalf("expected scope to be injected")
	}
	if scope["name"] != "gestalt/test" {
		t.Fatalf("expected scope name, got %v", scope["name"])
	}
}
