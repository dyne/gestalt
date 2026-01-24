package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type WorkflowEventsHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

type workflowEventPayload struct {
	Type       string         `json:"type"`
	WorkflowID string         `json:"workflow_id"`
	SessionID  string         `json:"session_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Context    map[string]any `json:"context,omitempty"`
}

func (h *WorkflowEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireWSToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	conn, err := upgradeWebSocket(w, r, h.AllowedOrigins)
	if err != nil {
		logWSError(h.Logger, r, wsError{
			Status:  http.StatusBadRequest,
			Message: "websocket upgrade failed",
			Err:     err,
		})
		return
	}
	if h.Manager == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "manager unavailable",
		})
		return
	}

	bus := h.Manager.WorkflowBus()
	if bus == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "workflow events unavailable",
		})
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "workflow events unavailable",
		})
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.WorkflowEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Conn:           conn,
		Logger:         h.Logger,
		Output:         output,
		BuildPayload: func(event eventtypes.WorkflowEvent) (any, bool) {
			payload := workflowEventPayload{
				Type:       event.Type(),
				WorkflowID: event.WorkflowID,
				SessionID:  event.SessionID,
				Timestamp:  event.Timestamp(),
				Context:    event.Context,
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})
}
