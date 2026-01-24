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

	bus := h.Manager.AgentBus()
	if bus == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "agent events unavailable",
		})
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "agent events unavailable",
		})
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.AgentEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Conn:           conn,
		Logger:         h.Logger,
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
