package api

import (
	"net/http"
	"time"

	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
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
	if !validateToken(r, h.AuthToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
