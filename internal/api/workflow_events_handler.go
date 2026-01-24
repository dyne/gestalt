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
	var bus *eventtypes.Bus[eventtypes.WorkflowEvent]
	reason := "workflow events unavailable"
	if h.Manager == nil {
		reason = "manager unavailable"
	} else {
		bus = h.Manager.WorkflowBus()
	}

	serveWSBusStream(w, r, wsBusStreamConfig[eventtypes.WorkflowEvent]{
		Logger:            h.Logger,
		AuthToken:         h.AuthToken,
		AllowedOrigins:    h.AllowedOrigins,
		Bus:               bus,
		UnavailableReason: reason,
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
