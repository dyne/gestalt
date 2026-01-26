package otel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogRecordsTailReturnsLatestRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")

	line1 := buildLogLine(
		buildLogRecord("1", "one"),
		buildLogRecord("2", "two"),
	)
	line2 := buildLogLine(
		buildLogRecord("3", "three"),
	)
	content := strings.Join([]string{line1, line2}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	records, err := ReadLogRecordsTail(path, WithTailMaxRecords(2))
	if err != nil {
		t.Fatalf("ReadLogRecordsTail error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if recordBody(records[0]) != "two" {
		t.Fatalf("expected second record, got %q", recordBody(records[0]))
	}
	if recordBody(records[1]) != "three" {
		t.Fatalf("expected last record, got %q", recordBody(records[1]))
	}
}

func TestReadLogRecordsTailSkipsPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "otel.json")

	line1 := buildLogLine(
		buildLogRecord("1", strings.Repeat("a", 200)),
	)
	line2 := buildLogLine(
		buildLogRecord("2", "tail"),
	)
	content := strings.Join([]string{line1, line2}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	maxBytes := int64(len(line2) + 5)
	records, err := ReadLogRecordsTail(path, WithTailMaxBytes(maxBytes))
	if err != nil {
		t.Fatalf("ReadLogRecordsTail error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if recordBody(records[0]) != "tail" {
		t.Fatalf("expected tail record, got %q", recordBody(records[0]))
	}
}

func buildLogLine(records ...string) string {
	return `{"resourceLogs":[{"scopeLogs":[{"logRecords":[` + strings.Join(records, ",") + `]}]}]}`
}

func buildLogRecord(timeUnixNano, body string) string {
	return `{"timeUnixNano":"` + timeUnixNano + `","severityText":"INFO","body":{"stringValue":"` + body + `"}}`
}

func recordBody(record map[string]any) string {
	body, ok := record["body"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := body["stringValue"].(string)
	return value
}
