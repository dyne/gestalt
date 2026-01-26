package otel

import (
	"strconv"
	"time"

	"gestalt/internal/logging"
)

const (
	severityDebugNumber   = 5
	severityInfoNumber    = 9
	severityWarningNumber = 13
	severityErrorNumber   = 17
)

func legacyEntryToOTLP(entry logging.LogEntry, resource map[string]any, scopeName string) map[string]any {
	timestamp := entry.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	observed := time.Now().UTC()

	return map[string]any{
		"timeUnixNano":         strconv.FormatInt(timestamp.UnixNano(), 10),
		"observedTimeUnixNano": strconv.FormatInt(observed.UnixNano(), 10),
		"severityNumber":       legacySeverityNumber(entry.Level),
		"severityText":         legacySeverityText(entry.Level),
		"body":                 map[string]any{"stringValue": entry.Message},
		"attributes":           legacyAttributes(entry.Context),
		"resource":             resourceFallback(resource),
		"scope":                scopeFromName(scopeName),
	}
}

func legacyAttributes(context map[string]string) []any {
	if len(context) == 0 {
		return []any{}
	}
	attributes := make([]any, 0, len(context))
	for key, value := range context {
		if key == "" {
			continue
		}
		attributes = append(attributes, map[string]any{
			"key": key,
			"value": map[string]any{
				"stringValue": value,
			},
		})
	}
	return attributes
}

func legacySeverityNumber(level logging.Level) int {
	switch level {
	case logging.LevelDebug:
		return severityDebugNumber
	case logging.LevelWarning:
		return severityWarningNumber
	case logging.LevelError:
		return severityErrorNumber
	default:
		return severityInfoNumber
	}
}

func legacySeverityText(level logging.Level) string {
	if level == "" {
		return string(logging.LevelInfo)
	}
	return string(level)
}

func scopeFromName(name string) map[string]any {
	if name == "" {
		return map[string]any{}
	}
	return map[string]any{"name": name}
}
