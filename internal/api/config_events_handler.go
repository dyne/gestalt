package api

import (
	"net/http"
	"time"

	"gestalt/internal/config"

	"github.com/gorilla/websocket"
)

type ConfigEventsHandler struct {
	AuthToken      string
	AllowedOrigins []string
}

type configEventPayload struct {
	Type       string    `json:"type"`
	ConfigType string    `json:"config_type"`
	Path       string    `json:"path"`
	ChangeType string    `json:"change_type"`
	Message    string    `json:"message,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

func (h *ConfigEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !validateToken(r, h.AuthToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	bus := config.Bus()
	if bus == nil {
		http.Error(w, "config events unavailable", http.StatusInternalServerError)
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		http.Error(w, "config events unavailable", http.StatusInternalServerError)
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
				payload := configEventPayload{
					Type:       event.Type(),
					ConfigType: event.ConfigType,
					Path:       event.Path,
					ChangeType: event.ChangeType,
					Message:    event.Message,
					Timestamp:  event.Timestamp(),
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
