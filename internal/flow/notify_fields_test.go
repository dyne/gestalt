package flow

import (
	"testing"
	"time"
)

func TestBuildNotifyFields(testingContext *testing.T) {
	occurredAt := time.Date(2026, 2, 1, 12, 30, 0, 123456000, time.UTC)
	input := NotifyFieldInput{
		SessionID:   "sess-1",
		AgentID:     "agent-1",
		AgentName:   "sess-1",
		EventID:     "event-123",
		PayloadType: "new-plan",
		OccurredAt:  occurredAt,
		Payload: map[string]any{
			"type":        "new-plan",
			"plan_file":   "PLAN.org",
			"task_level":  2,
			"task_state":  "WIP",
			"done":        true,
			"session_id":  "payload-session",
			"timestamp":   "2020-01-01T00:00:00Z",
			"extra_obj":   map[string]any{"a": "b"},
			"extra_array": []any{"x"},
		},
	}

	fields := BuildNotifyFields(input)

	if fields["type"] != "notify_new_plan" {
		testingContext.Fatalf("expected canonical type, got %q", fields["type"])
	}
	if fields["timestamp"] != occurredAt.Format(time.RFC3339Nano) {
		testingContext.Fatalf("unexpected timestamp %q", fields["timestamp"])
	}
	if fields["session_id"] != "sess-1" {
		testingContext.Fatalf("unexpected session_id %q", fields["session_id"])
	}
	if fields["agent_id"] != "agent-1" {
		testingContext.Fatalf("unexpected agent_id %q", fields["agent_id"])
	}
	if fields["agent_name"] != "sess-1" {
		testingContext.Fatalf("unexpected agent_name %q", fields["agent_name"])
	}
	if fields["notify.type"] != "new-plan" {
		testingContext.Fatalf("unexpected notify.type %q", fields["notify.type"])
	}
	if fields["notify.event_id"] != "event-123" {
		testingContext.Fatalf("unexpected notify.event_id %q", fields["notify.event_id"])
	}
	if fields["plan_file"] != "PLAN.org" || fields["notify.plan_file"] != "PLAN.org" {
		testingContext.Fatalf("expected plan_file aliases, got %q/%q", fields["plan_file"], fields["notify.plan_file"])
	}
	if fields["task_level"] != "2" || fields["notify.task_level"] != "2" {
		testingContext.Fatalf("expected task_level aliases, got %q/%q", fields["task_level"], fields["notify.task_level"])
	}
	if fields["task_state"] != "WIP" || fields["notify.task_state"] != "WIP" {
		testingContext.Fatalf("expected task_state aliases, got %q/%q", fields["task_state"], fields["notify.task_state"])
	}
	if fields["done"] != "true" || fields["notify.done"] != "true" {
		testingContext.Fatalf("expected done aliases, got %q/%q", fields["done"], fields["notify.done"])
	}
	if fields["notify.session_id"] != "payload-session" {
		testingContext.Fatalf("expected notify.session_id from payload, got %q", fields["notify.session_id"])
	}
	if fields["timestamp"] == "2020-01-01T00:00:00Z" || fields["notify.timestamp"] != "" {
		testingContext.Fatalf("reserved keys should not be aliased into notify.timestamp")
	}
	if _, ok := fields["extra_obj"]; ok {
		testingContext.Fatalf("unexpected object alias %q", fields["extra_obj"])
	}
	if _, ok := fields["extra_array"]; ok {
		testingContext.Fatalf("unexpected array alias %q", fields["extra_array"])
	}
}
