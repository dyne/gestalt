package api

import (
	"net/http"
	"time"

	"gestalt/internal/config"
	eventtypes "gestalt/internal/event"
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

	serveWSStream(w, r, wsStreamConfig[eventtypes.ConfigEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Output:         output,
		BuildPayload: func(event eventtypes.ConfigEvent) (any, bool) {
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
			return payload, true
		},
	})
}
