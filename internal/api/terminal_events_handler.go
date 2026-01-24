package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/terminal"
)

type TerminalEventsHandler struct {
	Manager        *terminal.Manager
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
	if !requireWSToken(w, r, h.AuthToken) {
		return
	}
	if h.Manager == nil {
		http.Error(w, "terminal manager unavailable", http.StatusInternalServerError)
		return
	}

	bus := h.Manager.TerminalBus()
	if bus == nil {
		http.Error(w, "terminal events unavailable", http.StatusInternalServerError)
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		http.Error(w, "terminal events unavailable", http.StatusInternalServerError)
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.TerminalEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Output:         output,
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
