package event

import (
	"testing"
	"time"
)

var _ Event = FileEvent{}
var _ Event = TerminalEvent{}
var _ Event = AgentEvent{}
var _ Event = ConfigEvent{}
var _ Event = WorkflowEvent{}
var _ Event = LogEvent{}

func TestNewFileEvent(t *testing.T) {
	event := NewFileEvent("/tmp/plan.org", "write")

	if event.Type() != "file_changed" {
		t.Fatalf("expected file_changed, got %q", event.Type())
	}
	if event.Path != "/tmp/plan.org" {
		t.Fatalf("expected path, got %q", event.Path)
	}
	if event.Operation != "write" {
		t.Fatalf("expected operation write, got %q", event.Operation)
	}
	assertUTC(t, event.Timestamp())
}

func TestNewTerminalEvent(t *testing.T) {
	event := NewTerminalEvent("term-1", "terminal_created")

	if event.Type() != "terminal_created" {
		t.Fatalf("expected terminal_created, got %q", event.Type())
	}
	if event.TerminalID != "term-1" {
		t.Fatalf("expected terminal ID, got %q", event.TerminalID)
	}
	assertUTC(t, event.Timestamp())
}

func TestNewAgentEvent(t *testing.T) {
	event := NewAgentEvent("agent-1", "Alex", "agent_started")

	if event.Type() != "agent_started" {
		t.Fatalf("expected agent_started, got %q", event.Type())
	}
	if event.AgentID != "agent-1" {
		t.Fatalf("expected agent ID, got %q", event.AgentID)
	}
	if event.AgentName != "Alex" {
		t.Fatalf("expected agent name, got %q", event.AgentName)
	}
	assertUTC(t, event.Timestamp())
}

func TestNewConfigEvent(t *testing.T) {
	event := NewConfigEvent("agent", "/config/agents/example.json", "modified")

	if event.Type() != "config_modified" {
		t.Fatalf("expected config_modified, got %q", event.Type())
	}
	if event.ConfigType != "agent" {
		t.Fatalf("expected config type agent, got %q", event.ConfigType)
	}
	if event.Path != "/config/agents/example.json" {
		t.Fatalf("expected path, got %q", event.Path)
	}
	if event.ChangeType != "modified" {
		t.Fatalf("expected change type modified, got %q", event.ChangeType)
	}
	assertUTC(t, event.Timestamp())
}

func TestNewWorkflowEvent(t *testing.T) {
	event := NewWorkflowEvent("workflow-1", "session-1", "workflow_paused")

	if event.Type() != "workflow_paused" {
		t.Fatalf("expected workflow_paused, got %q", event.Type())
	}
	if event.WorkflowID != "workflow-1" {
		t.Fatalf("expected workflow ID, got %q", event.WorkflowID)
	}
	if event.SessionID != "session-1" {
		t.Fatalf("expected session ID, got %q", event.SessionID)
	}
	assertUTC(t, event.Timestamp())
}

func TestNewLogEvent(t *testing.T) {
	context := map[string]string{"terminal": "1"}
	event := NewLogEvent("info", "hello", context)

	if event.Type() != "log_entry" {
		t.Fatalf("expected log_entry, got %q", event.Type())
	}
	if event.Level != "info" {
		t.Fatalf("expected level info, got %q", event.Level)
	}
	if event.Message != "hello" {
		t.Fatalf("expected message hello, got %q", event.Message)
	}
	if event.Context["terminal"] != "1" {
		t.Fatalf("expected context terminal 1, got %q", event.Context["terminal"])
	}
	assertUTC(t, event.Timestamp())
}

func assertUTC(t *testing.T, value time.Time) {
	t.Helper()
	if value.IsZero() {
		t.Fatal("expected timestamp to be set")
	}
	if value.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %v", value.Location())
	}
}
