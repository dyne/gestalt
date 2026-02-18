package notify

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/otel"
)

const (
	severityDebugNumber   = 5
	severityInfoNumber    = 9
	severityWarningNumber = 13
	severityErrorNumber   = 17
)

type OTelSink struct {
	hub *otel.LogHub
}

func NewOTelSink(hub *otel.LogHub) *OTelSink {
	if hub == nil {
		hub = otel.ActiveLogHub()
	}
	return &OTelSink{hub: hub}
}

func (sink *OTelSink) Emit(_ context.Context, event Event) error {
	if sink == nil || sink.hub == nil {
		return errors.New("notification sink unavailable")
	}
	record := buildOTelRecord(event)
	if record == nil {
		return errors.New("notification record unavailable")
	}
	sink.hub.Append(record)
	return nil
}

func buildOTelRecord(event Event) map[string]any {
	timestamp := event.OccurredAt
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	observed := time.Now().UTC()

	message := strings.TrimSpace(event.Message)
	if message == "" {
		message = strings.TrimSpace(event.Fields["notify.type"])
	}
	if message == "" {
		message = strings.TrimSpace(event.Fields["type"])
	}
	if message == "" {
		message = "notification"
	}

	fields := map[string]string{}
	for key, value := range event.Fields {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		fields[key] = value
	}
	if _, ok := fields["gestalt.category"]; !ok {
		fields["gestalt.category"] = "notification"
	}
	if _, ok := fields["gestalt.source"]; !ok {
		fields["gestalt.source"] = "backend"
	}

	return map[string]any{
		"timeUnixNano":         strconv.FormatInt(timestamp.UnixNano(), 10),
		"observedTimeUnixNano": strconv.FormatInt(observed.UnixNano(), 10),
		"severityNumber":       severityNumber(event.Level),
		"severityText":         severityText(event.Level),
		"body":                 map[string]any{"stringValue": message},
		"attributes":           otelAttributes(fields),
		"resource":             map[string]any{"attributes": []any{}},
		"scope":                map[string]any{"name": "gestalt/notify"},
	}
}

func severityNumber(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return severityDebugNumber
	case "warning", "warn":
		return severityWarningNumber
	case "error":
		return severityErrorNumber
	default:
		return severityInfoNumber
	}
}

func severityText(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return "info"
	}
	if level == "warn" {
		return "warning"
	}
	return level
}

func otelAttributes(fields map[string]string) []any {
	if len(fields) == 0 {
		return []any{}
	}
	attributes := make([]any, 0, len(fields))
	for key, value := range fields {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
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
