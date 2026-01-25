package api

import (
	"net/http"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
)

type SCIPEventsHandler struct {
	AuthToken      string
	AllowedOrigins []string
	Logger         *logging.Logger
	Bus            *eventtypes.Bus[eventtypes.SCIPEvent]
}

type scipEventPayload struct {
	Type      string    `json:"type"`
	Language  string    `json:"language,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func (h *SCIPEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveWSBusStream(w, r, wsBusStreamConfig[eventtypes.SCIPEvent]{
		Logger:            h.Logger,
		AuthToken:         h.AuthToken,
		AllowedOrigins:    h.AllowedOrigins,
		Bus:               h.Bus,
		UnavailableReason: "scip events unavailable",
		BuildPayload: func(event eventtypes.SCIPEvent) (any, bool) {
			payload := scipEventPayload{
				Type:      event.Type(),
				Language:  event.Language,
				Message:   event.Message,
				Timestamp: event.Timestamp(),
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})
}
