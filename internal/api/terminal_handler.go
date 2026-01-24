package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

type TerminalHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

// We keep gorilla/websocket because stdlib has no WebSocket server support and
// x/net/websocket is effectively frozen; gorilla provides a maintained upgrader,
// origin checks, and explicit binary/text frame handling.
// controlMessage is JSON-encoded in text frames to carry resize updates.
type controlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (h *TerminalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	defer conn.Close()

	if h.Manager == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal manager unavailable",
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/ws/terminal/")
	if id == "" || id == r.URL.Path {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusBadRequest,
			Message: "missing terminal id",
		})
		return
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusNotFound,
			Message: "terminal not found",
		})
		return
	}

	output, cancel := session.Subscribe()
	defer cancel()

	// Avoid duplicating scrollback for clients that already hydrate via /output.
	if len(session.OutputLines()) == 0 {
		historyLines, err := session.HistoryLines(terminal.DefaultHistoryLines)
		if err == nil && len(historyLines) > 0 {
			payload := strings.Join(historyLines, "\n")
			if payload != "" {
				if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte(payload)); err != nil {
					return
				}
			}
		}
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case chunk, ok := <-output:
				if !ok {
					return
				}
				if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch msgType {
		case websocket.TextMessage:
			if control, ok := parseControlMessage(msg); ok {
				if control.Type == "resize" {
					if err := session.Resize(control.Cols, control.Rows); err != nil {
						return
					}
					if bus := h.Manager.TerminalBus(); bus != nil {
						terminalEvent := event.NewTerminalEvent(session.ID, "terminal_resized")
						terminalEvent.Data = map[string]any{
							"cols": control.Cols,
							"rows": control.Rows,
						}
						bus.Publish(terminalEvent)
					}
				}
				continue
			}
			if err := session.Write(msg); err != nil {
				return
			}
		case websocket.BinaryMessage:
			if err := session.Write(msg); err != nil {
				return
			}
		}
	}
}

func parseControlMessage(data []byte) (controlMessage, bool) {
	var msg controlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return controlMessage{}, false
	}

	if msg.Type != "resize" {
		return msg, false
	}
	if msg.Cols == 0 || msg.Rows == 0 {
		return msg, false
	}

	return msg, true
}

func validateToken(r *http.Request, token string) bool {
	if token == "" {
		return true
	}

	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ") == token
	}

	queryToken := r.URL.Query().Get("token")
	if queryToken != "" {
		return queryToken == token
	}

	return false
}

func isOriginAllowed(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := parsed.Hostname()
	if originHost == "" {
		return false
	}

	if len(allowed) > 0 {
		for _, allowedOrigin := range allowed {
			if strings.EqualFold(origin, allowedOrigin) || strings.EqualFold(originHost, allowedOrigin) {
				return true
			}
		}
		return false
	}

	requestHost := hostOnly(r.Host)
	return strings.EqualFold(originHost, requestHost)
}

func hostOnly(hostport string) string {
	host := hostport
	if strings.HasPrefix(hostport, "[") {
		if parsedHost, _, err := net.SplitHostPort(hostport); err == nil {
			host = parsedHost
		}
		return strings.Trim(host, "[]")
	}

	if parsedHost, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsedHost
	}

	return host
}
