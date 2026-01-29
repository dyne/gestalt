package api

import (
	"context"
	"net/http"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/notification"
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

	bus := notification.Bus()
	if bus == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusServiceUnavailable, "notification stream unavailable")
		return
	}

	output, cancelSubscription := bus.Subscribe()
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

	runSSEStream(r, writer, sseStreamConfig[notification.Event]{
		Logger:    h.Logger,
		Output:    output,
		SkipRetry: true,
		BuildPayload: func(event notification.Event) (any, bool) {
			payload := notificationPayload{
				Type:      event.Type(),
				Level:     event.Level,
				Message:   event.Message,
				Timestamp: event.OccurredAt,
			}
			if payload.Type == "" {
				payload.Type = notification.EventTypeToast
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})

	cancel()
}
