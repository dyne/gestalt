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

	bus := config.Bus()
	if bus == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "config events unavailable",
		})
		return
	}
	output, cancel := bus.Subscribe()
	if output == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "config events unavailable",
		})
		return
	}
	defer cancel()

	serveWSStream(w, r, wsStreamConfig[eventtypes.ConfigEvent]{
		AllowedOrigins: h.AllowedOrigins,
		Conn:           conn,
		Logger:         h.Logger,
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
