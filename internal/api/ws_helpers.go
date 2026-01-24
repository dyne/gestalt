package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"

	"github.com/gorilla/websocket"
)

const wsReadBufferSize = 1024
const wsWriteBufferSize = 1024
const wsWriteTimeout = 10 * time.Second

type wsStreamConfig[T any] struct {
	AllowedOrigins []string
	Conn           *websocket.Conn
	Output         <-chan T
	BuildPayload   func(T) (any, bool)
	WriteTimeout   time.Duration
	Logger         *logging.Logger
}

type wsError struct {
	Status       int
	CloseCode    int
	Message      string
	Err          error
	SendEnvelope bool
}

type wsErrorPayload struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	Status    int    `json:"status"`
	CloseCode int    `json:"close_code,omitempty"`
}

func requireWSToken(w http.ResponseWriter, r *http.Request, token string, logger *logging.Logger) bool {
	if !validateToken(r, token) {
		writeWSError(w, r, nil, logger, wsError{
			Status:    http.StatusUnauthorized,
			CloseCode: websocket.ClosePolicyViolation,
			Message:   "unauthorized",
		})
		return false
	}
	return true
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request, allowedOrigins []string) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  wsReadBufferSize,
		WriteBufferSize: wsWriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return isOriginAllowed(r, allowedOrigins)
		},
	}
	return upgrader.Upgrade(w, r, nil)
}

func serveWSStream[T any](w http.ResponseWriter, r *http.Request, config wsStreamConfig[T]) {
	if config.Output == nil {
		return
	}

	conn := config.Conn
	if conn == nil {
		var err error
		conn, err = upgradeWebSocket(w, r, config.AllowedOrigins)
		if err != nil {
			logWSError(config.Logger, r, wsError{
				Status:  http.StatusBadRequest,
				Message: "websocket upgrade failed",
				Err:     err,
			})
			return
		}
	}
	defer conn.Close()

	writeTimeout := config.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = wsWriteTimeout
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case event, ok := <-config.Output:
				if !ok {
					return
				}
				payload, ok := config.BuildPayload(event)
				if !ok {
					continue
				}
				if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
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

// writeWSError sends a close frame when a websocket is available, falling back to HTTP errors otherwise.
func writeWSError(w http.ResponseWriter, r *http.Request, conn *websocket.Conn, logger *logging.Logger, wsErr wsError) {
	status := wsErr.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	reason := strings.TrimSpace(wsErr.Message)
	if reason == "" {
		reason = http.StatusText(status)
		if reason == "" {
			reason = "websocket error"
		}
	}
	closeCode := wsErr.CloseCode
	if closeCode == 0 {
		closeCode = closeCodeForStatus(status)
	}

	logWSError(logger, r, wsError{
		Status:    status,
		CloseCode: closeCode,
		Message:   reason,
		Err:       wsErr.Err,
	})

	if conn == nil {
		http.Error(w, reason, status)
		return
	}

	deadline := time.Now().Add(wsWriteTimeout)
	if wsErr.SendEnvelope {
		_ = conn.SetWriteDeadline(deadline)
		_ = conn.WriteJSON(wsErrorPayload{
			Type:      "error",
			Message:   reason,
			Status:    status,
			CloseCode: closeCode,
		})
	}
	_ = conn.SetWriteDeadline(deadline)
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, truncateCloseReason(reason)), deadline)
	_ = conn.Close()
}

func logWSError(logger *logging.Logger, r *http.Request, wsErr wsError) {
	if logger == nil || r == nil {
		return
	}

	closeCode := wsErr.CloseCode
	if closeCode == 0 {
		closeCode = closeCodeForStatus(wsErr.Status)
	}

	fields := map[string]string{
		"path":       r.URL.Path,
		"status":     strconv.Itoa(wsErr.Status),
		"close_code": strconv.Itoa(closeCode),
		"message":    wsErr.Message,
	}
	if r.RemoteAddr != "" {
		fields["remote_addr"] = r.RemoteAddr
	}
	if userAgent := strings.TrimSpace(r.UserAgent()); userAgent != "" {
		fields["user_agent"] = userAgent
	}
	if wsErr.Err != nil {
		fields["error"] = wsErr.Err.Error()
	}

	if wsErr.Status >= http.StatusInternalServerError {
		logger.Error("websocket error", fields)
	} else {
		logger.Warn("websocket error", fields)
	}
}

func closeCodeForStatus(status int) int {
	switch {
	case status == http.StatusBadRequest:
		return websocket.CloseProtocolError
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return websocket.ClosePolicyViolation
	case status == http.StatusServiceUnavailable:
		return websocket.CloseTryAgainLater
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return websocket.ClosePolicyViolation
	default:
		return websocket.CloseInternalServerErr
	}
}

func truncateCloseReason(reason string) string {
	const maxReasonBytes = 123
	if len(reason) <= maxReasonBytes {
		return reason
	}
	return reason[:maxReasonBytes]
}
