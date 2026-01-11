package event

import "time"

// Event represents a typed event with an occurrence timestamp.
type Event interface {
	Type() string
	Timestamp() time.Time
}

// FileEvent represents a filesystem change.
type FileEvent struct {
	EventType  string
	Path       string
	Operation  string
	OccurredAt time.Time
}

func NewFileEvent(path, operation string) FileEvent {
	return FileEvent{
		EventType:  "file_changed",
		Path:       path,
		Operation:  operation,
		OccurredAt: time.Now().UTC(),
	}
}

func (e FileEvent) Type() string {
	return e.EventType
}

func (e FileEvent) Timestamp() time.Time {
	return e.OccurredAt
}

// TerminalEvent captures terminal lifecycle changes.
type TerminalEvent struct {
	EventType  string
	TerminalID string
	Data       map[string]any
	OccurredAt time.Time
}

func NewTerminalEvent(terminalID, eventType string) TerminalEvent {
	return TerminalEvent{
		EventType:  eventType,
		TerminalID: terminalID,
		OccurredAt: time.Now().UTC(),
	}
}

func (e TerminalEvent) Type() string {
	return e.EventType
}

func (e TerminalEvent) Timestamp() time.Time {
	return e.OccurredAt
}

// AgentEvent captures agent lifecycle changes.
type AgentEvent struct {
	EventType  string
	AgentID    string
	AgentName  string
	Context    map[string]any
	OccurredAt time.Time
}

func NewAgentEvent(agentID, agentName, eventType string) AgentEvent {
	return AgentEvent{
		EventType:  eventType,
		AgentID:    agentID,
		AgentName:  agentName,
		OccurredAt: time.Now().UTC(),
	}
}

func (e AgentEvent) Type() string {
	return e.EventType
}

func (e AgentEvent) Timestamp() time.Time {
	return e.OccurredAt
}

// ConfigEvent captures config changes.
type ConfigEvent struct {
	EventType  string
	ConfigType string
	Path       string
	ChangeType string
	OccurredAt time.Time
}

func NewConfigEvent(configType, path, changeType string) ConfigEvent {
	eventType := "config_" + changeType
	return ConfigEvent{
		EventType:  eventType,
		ConfigType: configType,
		Path:       path,
		ChangeType: changeType,
		OccurredAt: time.Now().UTC(),
	}
}

func (e ConfigEvent) Type() string {
	return e.EventType
}

func (e ConfigEvent) Timestamp() time.Time {
	return e.OccurredAt
}

// WorkflowEvent captures workflow state changes.
type WorkflowEvent struct {
	EventType  string
	WorkflowID string
	SessionID  string
	OccurredAt time.Time
}

func NewWorkflowEvent(workflowID, sessionID, eventType string) WorkflowEvent {
	return WorkflowEvent{
		EventType:  eventType,
		WorkflowID: workflowID,
		SessionID:  sessionID,
		OccurredAt: time.Now().UTC(),
	}
}

func (e WorkflowEvent) Type() string {
	return e.EventType
}

func (e WorkflowEvent) Timestamp() time.Time {
	return e.OccurredAt
}

// LogEvent wraps log data for streaming.
type LogEvent struct {
	EventType  string
	Level      string
	Message    string
	Context    map[string]string
	OccurredAt time.Time
}

func NewLogEvent(level, message string, context map[string]string) LogEvent {
	return LogEvent{
		EventType:  "log_entry",
		Level:      level,
		Message:    message,
		Context:    context,
		OccurredAt: time.Now().UTC(),
	}
}

func (e LogEvent) Type() string {
	return e.EventType
}

func (e LogEvent) Timestamp() time.Time {
	return e.OccurredAt
}
