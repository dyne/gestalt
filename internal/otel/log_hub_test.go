package otel

import (
	"strconv"
	"testing"
	"time"
)

func TestLogHubSnapshotPrunesOldRecords(t *testing.T) {
	hub := NewLogHub(time.Minute)
	now := time.Now()
	oldRecord := logRecordAt(now.Add(-2*time.Minute), "old")
	newRecord := logRecordAt(now.Add(-30*time.Second), "new")

	hub.Append(oldRecord, newRecord)

	snapshot := hub.SnapshotSince(now.Add(-time.Minute))
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 record, got %d", len(snapshot))
	}
	if message(snapshot[0]) != "new" {
		t.Fatalf("expected newest record, got %q", message(snapshot[0]))
	}
}

func TestLogHubSubscribeOrdering(t *testing.T) {
	hub := NewLogHub(time.Hour)
	ch, cancel := hub.Subscribe()
	defer cancel()

	first := logRecordAt(time.Now(), "first")
	second := logRecordAt(time.Now().Add(time.Second), "second")

	hub.Append(first)
	hub.Append(second)

	if message(receiveRecord(t, ch)) != "first" {
		t.Fatalf("expected first record")
	}
	if message(receiveRecord(t, ch)) != "second" {
		t.Fatalf("expected second record")
	}
}

func TestLogHubUsesObservedTimeWhenMissing(t *testing.T) {
	hub := NewLogHub(time.Minute)
	now := time.Now()
	record := map[string]any{
		"observedTimeUnixNano": strconv.FormatInt(now.UnixNano(), 10),
		"body": map[string]any{"stringValue": "observed"},
	}
	hub.Append(record)

	snapshot := hub.SnapshotSince(now.Add(-time.Minute))
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 record, got %d", len(snapshot))
	}
	if message(snapshot[0]) != "observed" {
		t.Fatalf("expected observed record, got %q", message(snapshot[0]))
	}
}

func receiveRecord(t *testing.T, ch <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case record := <-ch:
		return record
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for record")
		return nil
	}
}

func logRecordAt(timestamp time.Time, messageText string) map[string]any {
	return map[string]any{
		"timeUnixNano": strconv.FormatInt(timestamp.UnixNano(), 10),
		"body":         map[string]any{"stringValue": messageText},
	}
}

func message(record map[string]any) string {
	body, ok := record["body"].(map[string]any)
	if !ok {
		return ""
	}
	if value, ok := body["stringValue"].(string); ok {
		return value
	}
	return ""
}
