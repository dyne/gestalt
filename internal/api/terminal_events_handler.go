package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type TerminalEventsHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

type terminalEventPayload struct {
	Type       string         `json:"type"`
	TerminalID string         `json:"terminal_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Data       map[string]any `json:"data,omitempty"`
}

func (h *TerminalEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var bus *eventtypes.Bus[eventtypes.TerminalEvent]
	reason := "terminal events unavailable"
	if h.Manager == nil {
		reason = "terminal manager unavailable"
	} else {
		bus = h.Manager.TerminalBus()
	}

	serveWSBusStream(w, r, wsBusStreamConfig[eventtypes.TerminalEvent]{
		Logger:            h.Logger,
		AuthToken:         h.AuthToken,
		AllowedOrigins:    h.AllowedOrigins,
		Bus:               bus,
		UnavailableReason: reason,
		BuildPayload: func(event eventtypes.TerminalEvent) (any, bool) {
			payload := terminalEventPayload{
				Type:       event.Type(),
				TerminalID: event.TerminalID,
				Timestamp:  event.Timestamp(),
				Data:       event.Data,
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})
}
