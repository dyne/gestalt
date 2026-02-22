package event

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	otellog "go.opentelemetry.io/otel/log"
)

var timeType = reflect.TypeOf(time.Time{})

func (b *Bus[T]) emitOTelEvent(event T, fallbackType string) {
	if b == nil || b.otelLogger == nil {
		return
	}

	eventName, eventTime, body, attrs, ok := eventLogData(event, fallbackType, b.busName())
	if !ok {
		return
	}

	severity, severityText := severityForEvent(eventName)
	ctx := context.Background()
	if !b.otelLogger.Enabled(ctx, otellog.EnabledParameters{Severity: severity, EventName: eventName}) {
		return
	}

	var record otellog.Record
	record.SetEventName(eventName)
	record.SetTimestamp(eventTime)
	record.SetObservedTimestamp(time.Now().UTC())
	record.SetSeverity(severity)
	record.SetSeverityText(severityText)
	record.SetBody(otellog.StringValue(body))
	if len(attrs) > 0 {
		record.AddAttributes(attrs...)
	}
	b.otelLogger.Emit(ctx, record)
}

func severityForEvent(eventName string) (otellog.Severity, string) {
	switch eventName {
	case "terminal-resized":
		return otellog.SeverityDebug, "debug"
	default:
		return otellog.SeverityInfo, "info"
	}
}

func eventLogData[T any](event T, fallbackType, busName string) (string, time.Time, string, []otellog.KeyValue, bool) {
	attrs := make([]otellog.KeyValue, 0, 8)
	if busName != "" {
		attrs = append(attrs, otellog.String("event.bus", busName))
	}

	var (
		eventName string
		eventTime time.Time
		body      string
	)

	if typed, ok := any(event).(Event); ok {
		eventName = strings.TrimSpace(typed.Type())
		eventTime = typed.Timestamp()
		attrs = append(attrs, eventAttributes(typed)...)
		body = eventBody(typed)
	}

	if eventName == "" && fallbackType != "" && fallbackType != "unknown" {
		eventName = fallbackType
	}

	if eventName == "" {
		name, timestamp, extra, ok := eventFromFields(event)
		if ok {
			eventName = name
			eventTime = timestamp
			attrs = append(attrs, extra...)
		}
	}

	if eventName == "" || eventName == "unknown" {
		return "", time.Time{}, "", nil, false
	}

	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}

	if body == "" {
		body = eventName
	}

	attrs = append(attrs, otellog.String("event.type", eventName))
	attrs = append(attrs, otellog.String("event.kind", fmt.Sprintf("%T", event)))
	return eventName, eventTime, body, attrs, true
}

func eventBody(event Event) string {
	switch typed := event.(type) {
	case LogEvent:
		return strings.TrimSpace(typed.Message)
	case *LogEvent:
		if typed == nil {
			return ""
		}
		return strings.TrimSpace(typed.Message)
	default:
		return ""
	}
}

func eventAttributes(event Event) []otellog.KeyValue {
	switch typed := event.(type) {
	case FileEvent:
		return fileEventAttributes(typed.Path, typed.Operation)
	case *FileEvent:
		if typed == nil {
			return nil
		}
		return fileEventAttributes(typed.Path, typed.Operation)
	case TerminalEvent:
		return terminalEventAttributes(typed)
	case *TerminalEvent:
		if typed == nil {
			return nil
		}
		return terminalEventAttributes(*typed)
	case AgentEvent:
		return agentEventAttributes(typed)
	case *AgentEvent:
		if typed == nil {
			return nil
		}
		return agentEventAttributes(*typed)
	case ConfigEvent:
		return configEventAttributes(typed)
	case *ConfigEvent:
		if typed == nil {
			return nil
		}
		return configEventAttributes(*typed)
	case WorkflowEvent:
		return workflowEventAttributes(typed)
	case *WorkflowEvent:
		if typed == nil {
			return nil
		}
		return workflowEventAttributes(*typed)
	case LogEvent:
		return logEventAttributes(typed)
	case *LogEvent:
		if typed == nil {
			return nil
		}
		return logEventAttributes(*typed)
	default:
		return nil
	}
}

func fileEventAttributes(path, operation string) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 2)
	if strings.TrimSpace(path) != "" {
		attrs = append(attrs, otellog.String("file.path", path))
	}
	if strings.TrimSpace(operation) != "" {
		attrs = append(attrs, otellog.String("file.operation", operation))
	}
	return attrs
}

func terminalEventAttributes(event TerminalEvent) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 4)
	if strings.TrimSpace(event.TerminalID) != "" {
		attrs = append(attrs, otellog.String("terminal.id", event.TerminalID))
	}
	return appendAnyMap(attrs, "terminal.data.", event.Data)
}

func agentEventAttributes(event AgentEvent) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 4)
	if strings.TrimSpace(event.AgentID) != "" {
		attrs = append(attrs, otellog.String("agent.id", event.AgentID))
	}
	if strings.TrimSpace(event.AgentName) != "" {
		attrs = append(attrs, otellog.String("agent.name", event.AgentName))
	}
	return appendAnyMap(attrs, "agent.context.", event.Context)
}

func configEventAttributes(event ConfigEvent) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 4)
	if strings.TrimSpace(event.ConfigType) != "" {
		attrs = append(attrs, otellog.String("config.type", event.ConfigType))
	}
	if strings.TrimSpace(event.Path) != "" {
		attrs = append(attrs, otellog.String("config.path", event.Path))
	}
	if strings.TrimSpace(event.ChangeType) != "" {
		attrs = append(attrs, otellog.String("config.change", event.ChangeType))
	}
	if strings.TrimSpace(event.Message) != "" {
		attrs = append(attrs, otellog.String("config.message", event.Message))
	}
	return attrs
}

func workflowEventAttributes(event WorkflowEvent) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 4)
	if strings.TrimSpace(event.WorkflowID) != "" {
		attrs = append(attrs, otellog.String("workflow.id", event.WorkflowID))
	}
	if strings.TrimSpace(event.SessionID) != "" {
		attrs = append(attrs, otellog.String("workflow.session_id", event.SessionID))
	}
	return appendAnyMap(attrs, "workflow.context.", event.Context)
}

func logEventAttributes(event LogEvent) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, 3)
	if strings.TrimSpace(event.Level) != "" {
		attrs = append(attrs, otellog.String("log.level", event.Level))
	}
	return appendStringMap(attrs, "log.context.", event.Context)
}

func eventFromFields[T any](event T) (string, time.Time, []otellog.KeyValue, bool) {
	value := reflect.ValueOf(event)
	if !value.IsValid() {
		return "", time.Time{}, nil, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", time.Time{}, nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return "", time.Time{}, nil, false
	}

	typeField := value.FieldByName("Type")
	if !typeField.IsValid() || typeField.Kind() != reflect.String {
		return "", time.Time{}, nil, false
	}
	eventName := strings.TrimSpace(typeField.String())
	if eventName == "" {
		return "", time.Time{}, nil, false
	}

	eventTime := time.Now().UTC()
	timestampField := value.FieldByName("Timestamp")
	if timestampField.IsValid() && timestampField.Type() == timeType {
		eventTime = timestampField.Interface().(time.Time)
	}

	attrs := make([]otellog.KeyValue, 0, 4)
	pathField := value.FieldByName("Path")
	if pathField.IsValid() && pathField.Kind() == reflect.String {
		path := strings.TrimSpace(pathField.String())
		if path != "" {
			attrs = append(attrs, otellog.String("file.path", path))
		}
	}
	opField := value.FieldByName("Op")
	if opField.IsValid() && opField.CanInterface() {
		attrs = append(attrs, otellog.String("file.operation", fmt.Sprint(opField.Interface())))
	}
	operationField := value.FieldByName("Operation")
	if operationField.IsValid() && operationField.Kind() == reflect.String {
		operation := strings.TrimSpace(operationField.String())
		if operation != "" {
			attrs = append(attrs, otellog.String("file.operation", operation))
		}
	}

	return eventName, eventTime, attrs, true
}

func appendStringMap(attrs []otellog.KeyValue, prefix string, values map[string]string) []otellog.KeyValue {
	if len(values) == 0 {
		return attrs
	}
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		attrs = append(attrs, otellog.String(prefix+trimmedKey, value))
	}
	return attrs
}

func appendAnyMap(attrs []otellog.KeyValue, prefix string, values map[string]any) []otellog.KeyValue {
	if len(values) == 0 {
		return attrs
	}
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || value == nil {
			continue
		}
		attrs = append(attrs, anyAttribute(prefix+trimmedKey, value))
	}
	return attrs
}

func anyAttribute(key string, value any) otellog.KeyValue {
	switch typed := value.(type) {
	case string:
		return otellog.String(key, typed)
	case bool:
		return otellog.Bool(key, typed)
	case int:
		return otellog.Int(key, typed)
	case int8:
		return otellog.Int64(key, int64(typed))
	case int16:
		return otellog.Int64(key, int64(typed))
	case int32:
		return otellog.Int64(key, int64(typed))
	case int64:
		return otellog.Int64(key, typed)
	case uint:
		return otellog.Int64(key, int64(typed))
	case uint8:
		return otellog.Int64(key, int64(typed))
	case uint16:
		return otellog.Int64(key, int64(typed))
	case uint32:
		return otellog.Int64(key, int64(typed))
	case uint64:
		return otellog.Int64(key, int64(typed))
	case float32:
		return otellog.Float64(key, float64(typed))
	case float64:
		return otellog.Float64(key, typed)
	case time.Time:
		return otellog.String(key, typed.Format(time.RFC3339Nano))
	case fmt.Stringer:
		return otellog.String(key, typed.String())
	case error:
		return otellog.String(key, typed.Error())
	default:
		return otellog.String(key, fmt.Sprint(value))
	}
}
