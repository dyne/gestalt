package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/terminal"
)

type AgentEventsHandler struct {
	Manager        *terminal.Manager
	AuthToken      string
	AllowedOrigins []string
}

type agentEventPayload struct {
	Type      string         `json:"type"`
	AgentID   string         `json:"agent_id"`
	AgentName string         `json:"agent_name"`
	Timestamp time.Time      `json:"timestamp"`
	Context   map[string]any `json:"context,omitempty"`
}

func (h *AgentEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !validateToken(r, h.AuthToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.Manager == nil {
		http.Error(w, "manager unavailable", http.StatusInternalServerError)
		return
	}

	bus := h.Manager.AgentBus()
	if bus == nil {
		http.Error(w, "agent events unavailable", http.StatusInternalServerError)
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		http.Error(w, "agent events unavailable", http.StatusInternalServerError)
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.AgentEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Output:         output,
		BuildPayload: func(event eventtypes.AgentEvent) (any, bool) {
			payload := agentEventPayload{
				Type:      event.Type(),
				AgentID:   event.AgentID,
				AgentName: event.AgentName,
				Timestamp: event.Timestamp(),
				Context:   event.Context,
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})
}
