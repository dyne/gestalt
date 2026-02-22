package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
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

	spanCtx, span := startWebSocketSpan(r, "/ws/session/:id")
	defer span.End()
	r = r.WithContext(spanCtx)

	if h.Manager == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal manager unavailable",
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/ws/session/")
	if id == "" || id == r.URL.Path {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusBadRequest,
			Message: "missing terminal id",
		})
		return
	}
	span.SetAttributes(attribute.String("session.id", id))

	session, ok := h.Manager.Get(id)
	if !ok {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusNotFound,
			Message: "terminal not found",
		})
		return
	}
	if strings.EqualFold(strings.TrimSpace(session.Runner), "external") {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "tmux-managed external session"),
			time.Now().Add(2*time.Second),
		)
		return
	}

	cursor, ok := parseCursorParam(r)
	if ok {
		if err := streamSessionLogFromCursor(conn, session, cursor); err != nil {
			return
		}
	}

	output, cancel := session.Subscribe()
	defer cancel()
	writer, err := startWSWriteLoop(w, r, wsStreamConfig[[]byte]{
		Conn:         conn,
		Output:       output,
		WritePayload: writeBinaryPayload,
	})
	if err != nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal stream unavailable",
			Err:     err,
		})
		return
	}
	defer writer.Stop()

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
						terminalEvent := event.NewTerminalEvent(session.ID, "terminal-resized")
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

func parseCursorParam(r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if raw == "" {
		return 0, false
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, false
	}
	return cursor, true
}

func streamSessionLogFromCursor(conn *websocket.Conn, session *terminal.Session, cursor int64) error {
	if conn == nil || session == nil || cursor < 0 {
		return nil
	}
	path := session.LogPath()
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	offset := cursor
	for i := 0; i < 3; i++ {
		info, err := file.Stat()
		if err != nil {
			return err
		}
		size := info.Size()
		if size < offset {
			offset = size
		}
		if size > offset {
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			if err := streamFileRange(conn, file, size-offset); err != nil {
				return err
			}
			offset = size
		}

		info, err = file.Stat()
		if err != nil {
			return err
		}
		if info.Size() <= offset {
			break
		}
	}

	return nil
}

func streamFileRange(conn *websocket.Conn, file *os.File, length int64) error {
	if length <= 0 {
		return nil
	}
	const chunkSize = 32 * 1024
	remaining := length
	buffer := make([]byte, chunkSize)
	for remaining > 0 {
		readSize := chunkSize
		if int64(readSize) > remaining {
			readSize = int(remaining)
		}
		n, err := file.Read(buffer[:readSize])
		if n > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return err
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
				return err
			}
			remaining -= int64(n)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	return nil
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
