package api

import (
	"net/http"
	"time"

	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
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
	if !validateToken(r, h.AuthToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return isOriginAllowed(r, h.AllowedOrigins)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case event, ok := <-output:
				if !ok {
					return
				}
				payload := terminalEventPayload{
					Type:       event.Type(),
					TerminalID: event.TerminalID,
					Timestamp:  event.Timestamp(),
					Data:       event.Data,
				}
				if payload.Timestamp.IsZero() {
					payload.Timestamp = time.Now().UTC()
				}
				if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				if err := conn.WriteJSON(payload); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
