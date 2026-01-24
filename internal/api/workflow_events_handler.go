package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/terminal"
)

type WorkflowEventsHandler struct {
	Manager        *terminal.Manager
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
	if !requireWSToken(w, r, h.AuthToken) {
		return
	}
	if h.Manager == nil {
		http.Error(w, "manager unavailable", http.StatusInternalServerError)
		return
	}

	bus := h.Manager.WorkflowBus()
	if bus == nil {
		http.Error(w, "workflow events unavailable", http.StatusInternalServerError)
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		http.Error(w, "workflow events unavailable", http.StatusInternalServerError)
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.WorkflowEvent]{
		AllowedOrigins: h.AllowedOrigins,
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
