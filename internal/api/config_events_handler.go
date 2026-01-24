package api

import (
	"net/http"
	"time"

	"gestalt/internal/config"
	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
)

type ConfigEventsHandler struct {
	AuthToken      string
	AllowedOrigins []string
	Logger         *logging.Logger
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
	serveWSBusStream(w, r, wsBusStreamConfig[eventtypes.ConfigEvent]{
		Logger:            h.Logger,
		AuthToken:         h.AuthToken,
		AllowedOrigins:    h.AllowedOrigins,
		Bus:               config.Bus(),
		UnavailableReason: "config events unavailable",
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
