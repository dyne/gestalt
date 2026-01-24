package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type AgentEventsHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
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
	var bus *eventtypes.Bus[eventtypes.AgentEvent]
	reason := "agent events unavailable"
	if h.Manager == nil {
		reason = "manager unavailable"
	} else {
		bus = h.Manager.AgentBus()
	}

	serveWSBusStream(w, r, wsBusStreamConfig[eventtypes.AgentEvent]{
		Logger:            h.Logger,
		AuthToken:         h.AuthToken,
		AllowedOrigins:    h.AllowedOrigins,
		Bus:               bus,
		UnavailableReason: reason,
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
