package api

import (
	"net/http"
	"strings"

	"gestalt/internal/event"
	"gestalt/internal/logging"
)

type wsBusStreamConfig[T any] struct {
	Logger            *logging.Logger
	AuthToken         string
	AllowedOrigins    []string
	Bus               *event.Bus[T]
	UnavailableReason string
	BuildPayload      func(T) (any, bool)
}

// serveWSBusStream subscribes to a bus and streams payloads to a websocket connection.
func serveWSBusStream[T any](w http.ResponseWriter, r *http.Request, config wsBusStreamConfig[T]) {
	if !requireWSToken(w, r, config.AuthToken, config.Logger) {
		return
	}

	bus := config.Bus
	if bus == nil {
		writeWSError(w, r, nil, config.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      unavailableReason(config.UnavailableReason),
			SendEnvelope: true,
		})
		return
	}

	output, cancel := bus.Subscribe()
	if output == nil {
		writeWSError(w, r, nil, config.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      unavailableReason(config.UnavailableReason),
			SendEnvelope: true,
		})
		return
	}

	conn, err := upgradeWebSocket(w, r, config.AllowedOrigins)
	if err != nil {
		cancel()
		logWSError(config.Logger, r, wsError{
			Status:  http.StatusBadRequest,
			Message: "websocket upgrade failed",
			Err:     err,
		})
		return
	}
	defer cancel()

	spanCtx, span := startWebSocketSpan(r, r.URL.Path)
	defer span.End()
	r = r.WithContext(spanCtx)

	serveWSStream(w, r, wsStreamConfig[T]{
		AllowedOrigins: config.AllowedOrigins,
		Conn:           conn,
		Logger:         config.Logger,
		Output:         output,
		BuildPayload:   config.BuildPayload,
	})
}

func unavailableReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "event stream unavailable"
	}
	return reason
}
