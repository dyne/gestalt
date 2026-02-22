package flow

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type NotifyFieldInput struct {
	SessionID   string
	EventID     string
	PayloadType string
	OccurredAt  time.Time
	Payload     map[string]any
}

func BuildNotifyFields(input NotifyFieldInput) map[string]string {
	fields := map[string]string{}
	setNotifyField(fields, "type", CanonicalNotifyEventType(input.PayloadType), true)
	if !input.OccurredAt.IsZero() {
		setNotifyField(fields, "timestamp", input.OccurredAt.UTC().Format(time.RFC3339Nano), true)
	}
	setNotifyField(fields, "session_id", input.SessionID, true)
	setNotifyField(fields, "session.id", input.SessionID, true)
	setNotifyField(fields, "notify.type", input.PayloadType, true)
	if strings.TrimSpace(input.EventID) != "" {
		setNotifyField(fields, "notify.event_id", strings.TrimSpace(input.EventID), true)
	}

	for key, value := range input.Payload {
		normalized := normalizeNotifyKey(key)
		if normalized == "" || isReservedNotifyKey(normalized) {
			continue
		}
		if parsed, ok := stringifyNotifyValue(value); ok {
			setNotifyField(fields, "notify."+normalized, parsed, true)
			setNotifyField(fields, normalized, parsed, false)
		}
	}

	return fields
}

func normalizeNotifyKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func isReservedNotifyKey(key string) bool {
	switch key {
	case "type", "occurred_at", "timestamp", "agent_id", "agent_name", "agent.id", "agent.name":
		return true
	default:
		return false
	}
}

func stringifyNotifyValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", false
		}
		return typed, true
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
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func setNotifyField(fields map[string]string, key string, value string, overwrite bool) {
	key = normalizeNotifyKey(key)
	if key == "" {
		return
	}
	if strings.TrimSpace(value) == "" {
		return
	}
	if !overwrite {
		if _, exists := fields[key]; exists {
			return
		}
	}
	fields[key] = value
}
