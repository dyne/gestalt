package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
)

type NotificationsSSEHandler struct {
	Logger    *logging.Logger
	AuthToken string
}

type notificationPayload struct {
	Type      string    `json:"type"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func (h *NotificationsSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireSSEToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	spanCtx, span := startSSESpan(r, "/api/notifications/stream")
	defer span.End()

	ctx, cancel := context.WithCancel(spanCtx)
	defer cancel()
	r = r.WithContext(ctx)

	hub := otel.ActiveLogHub()
	if hub == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusServiceUnavailable, "notification stream unavailable")
		return
	}

	output, cancelSubscription := hub.Subscribe()
	if output == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusServiceUnavailable, "notification stream unavailable")
		return
	}
	defer cancelSubscription()

	writer, err := startSSEWriter(w)
	if err != nil {
		logSSEError(h.Logger, r, sseError{
			Status:  http.StatusInternalServerError,
			Message: "notification stream unavailable",
			Err:     err,
		})
		return
	}

	if err := writer.WriteRetry(defaultSSERetryInterval); err != nil {
		return
	}

	runSSEStream(r, writer, sseStreamConfig[map[string]any]{
		Logger:    h.Logger,
		Output:    output,
		SkipRetry: true,
		BuildPayload: func(record map[string]any) (any, bool) {
			payload, ok := notificationPayloadFromRecord(record)
			if !ok {
				return nil, false
			}
			return payload, true
		},
	})

	cancel()
}

func notificationPayloadFromRecord(record map[string]any) (notificationPayload, bool) {
	if record == nil {
		return notificationPayload{}, false
	}
	attributes := notificationAttributes(record)
	if category := attributes["gestalt.category"]; category != "" && category != "notification" {
		return notificationPayload{}, false
	}
	if attributes["gestalt.category"] == "" && attributes["notify.type"] == "" {
		return notificationPayload{}, false
	}

	payload := notificationPayload{
		Type:      strings.TrimSpace(attributes["notify.type"]),
		Level:     strings.TrimSpace(attributes["notify.level"]),
		Message:   logBodyString(record),
		Timestamp: notificationTimestamp(record),
	}
	if payload.Type == "" {
		payload.Type = strings.TrimSpace(attributes["type"])
	}
	if payload.Type == "" {
		payload.Type = "toast"
	}
	if payload.Level == "" {
		payload.Level = string(otelLogLevel(record))
	}
	if payload.Level == "" {
		payload.Level = string(logging.LevelInfo)
	}
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now().UTC()
	}
	return payload, true
}

func notificationTimestamp(record map[string]any) time.Time {
	if ts, ok := extractTimestamp(record, "timeUnixNano", "observedTimeUnixNano"); ok {
		return ts
	}
	return time.Time{}
}

func notificationAttributes(record map[string]any) map[string]string {
	attributes := map[string]string{}
	for _, entry := range asSlice(record["attributes"]) {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		key, _ := entryMap["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		valueMap, ok := entryMap["value"].(map[string]any)
		if !ok {
			continue
		}
		stringValue, _ := valueMap["stringValue"].(string)
		stringValue = strings.TrimSpace(stringValue)
		if stringValue == "" {
			continue
		}
		attributes[key] = stringValue
	}
	return attributes
}

func logBodyString(record map[string]any) string {
	body, ok := record["body"]
	if !ok || body == nil {
		return ""
	}
	switch typed := body.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if value, ok := typed["stringValue"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
