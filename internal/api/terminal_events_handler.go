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
			Message: "terminal manager unavailable",
		})
		return
	}

	bus := h.Manager.TerminalBus()
	if bus == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal events unavailable",
		})
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal events unavailable",
		})
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.TerminalEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Conn:           conn,
		Logger:         h.Logger,
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
