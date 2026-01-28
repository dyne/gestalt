package flow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	eventpkg "gestalt/internal/event"
	"gestalt/internal/watcher"
)

func NormalizeEvent(source any) map[string]string {
	switch event := source.(type) {
	case watcher.Event:
		return NormalizeWatcherEvent(event)
	case *watcher.Event:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeWatcherEvent(*event)
	case eventpkg.FileEvent:
		return NormalizeFileEvent(event)
	case *eventpkg.FileEvent:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeFileEvent(*event)
	case eventpkg.ConfigEvent:
		return NormalizeConfigEvent(event)
	case *eventpkg.ConfigEvent:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeConfigEvent(*event)
	case eventpkg.WorkflowEvent:
		return NormalizeWorkflowEvent(event)
	case *eventpkg.WorkflowEvent:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeWorkflowEvent(*event)
	case eventpkg.AgentEvent:
		return NormalizeAgentEvent(event)
	case *eventpkg.AgentEvent:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeAgentEvent(*event)
	case eventpkg.TerminalEvent:
		return NormalizeTerminalEvent(event)
	case *eventpkg.TerminalEvent:
		if event == nil {
			return map[string]string{}
		}
		return NormalizeTerminalEvent(*event)
	default:
		return map[string]string{}
	}
}

func NormalizeWatcherEvent(event watcher.Event) map[string]string {
	fields := baseFields(event.Type, event.Timestamp)
	setField(fields, "path", event.Path)
	if op := strings.TrimSpace(event.Op.String()); op != "" {
		setField(fields, "op", op)
	}
	return fields
}

func NormalizeFileEvent(event eventpkg.FileEvent) map[string]string {
	fields := baseFields(event.EventType, event.OccurredAt)
	setField(fields, "path", event.Path)
	setField(fields, "op", event.Operation)
	return fields
}

func NormalizeConfigEvent(event eventpkg.ConfigEvent) map[string]string {
	fields := baseFields(event.EventType, event.OccurredAt)
	setField(fields, "config_type", event.ConfigType)
	setField(fields, "path", event.Path)
	setField(fields, "change_type", event.ChangeType)
	setField(fields, "message", event.Message)
	return fields
}

func NormalizeWorkflowEvent(event eventpkg.WorkflowEvent) map[string]string {
	fields := baseFields(event.EventType, event.OccurredAt)
	setField(fields, "workflow_id", event.WorkflowID)
	setField(fields, "session_id", event.SessionID)
	addMapFields(fields, "context.", event.Context)
	return fields
}

func NormalizeAgentEvent(event eventpkg.AgentEvent) map[string]string {
	fields := baseFields(event.EventType, event.OccurredAt)
	setField(fields, "agent_id", event.AgentID)
	setField(fields, "agent_name", event.AgentName)
	addMapFields(fields, "context.", event.Context)
	return fields
}

func NormalizeTerminalEvent(event eventpkg.TerminalEvent) map[string]string {
	fields := baseFields(event.EventType, event.OccurredAt)
	setField(fields, "terminal_id", event.TerminalID)
	addMapFields(fields, "data.", event.Data)
	return fields
}

func baseFields(eventType string, timestamp time.Time) map[string]string {
	fields := map[string]string{}
	setField(fields, "type", eventType)
	if !timestamp.IsZero() {
		setField(fields, "timestamp", timestamp.UTC().Format(time.RFC3339Nano))
	}
	return fields
}

func addMapFields(fields map[string]string, prefix string, values map[string]any) {
	for key, value := range values {
		if key == "" {
			continue
		}
		if parsed, ok := stringifyValue(value); ok {
			setField(fields, prefix+key, parsed)
		}
	}
}

func setField(fields map[string]string, key string, value string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	if value == "" {
		return
	}
	fields[key] = value
}

func stringifyValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, typed != ""
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case time.Time:
		if typed.IsZero() {
			return "", false
		}
		return typed.UTC().Format(time.RFC3339Nano), true
	case fmt.Stringer:
		serialized := typed.String()
		return serialized, serialized != ""
	default:
		return "", false
	}
}
