package api

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/runner/proto"
	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
)

// RunnerHandler bridges external runners over websocket.
type RunnerHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

func (h *RunnerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	spanCtx, span := startWebSocketSpan(r, "/ws/runner/session/:id")
	defer span.End()
	r = r.WithContext(spanCtx)

	if h.Manager == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  http.StatusInternalServerError,
			Message: "terminal manager unavailable",
		})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/ws/runner/session/")
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

	writeMu := &sync.Mutex{}
	sendBinary := func(payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, payload)
	}
	sendControl := func(message interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
			return err
		}
		return conn.WriteJSON(message)
	}
	closeRunner := func() error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "runner closed"))
		return conn.Close()
	}

	attachErr := session.AttachExternalRunner(sendBinary, func(cols, rows uint16) error {
		return sendControl(proto.ResizeMessage{
			Type: proto.ControlTypeResize,
			Cols: cols,
			Rows: rows,
		})
	}, closeRunner)
	if attachErr != nil {
		status := http.StatusBadRequest
		message := "external runner unavailable"
		if errors.Is(attachErr, terminal.ErrRunnerAttached) {
			status = http.StatusConflict
			message = "runner already attached"
		}
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:  status,
			Message: message,
			Err:     attachErr,
		})
		return
	}
	defer session.DetachExternalRunner()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			session.PublishOutputChunk(msg)
		case websocket.TextMessage:
			if err := handleRunnerControlMessage(session, sendControl, msg); err != nil {
				return
			}
		}
	}
}

func handleRunnerControlMessage(session *terminal.Session, sendControl func(any) error, payload []byte) error {
	msg, err := proto.DecodeControlMessage(payload)
	if err != nil {
		return err
	}

	switch typed := msg.(type) {
	case proto.HelloMessage:
		if typed.ProtocolVersion != proto.ProtocolVersion {
			return errors.New("runner protocol version mismatch")
		}
	case proto.PingMessage:
		return sendControl(proto.PongMessage{Type: proto.ControlTypePong})
	case proto.PongMessage:
		return nil
	case proto.ExitMessage:
		if session != nil {
			_ = session.Close()
		}
	case proto.ResizeMessage:
		return nil
	default:
		return errors.New("unsupported runner control message")
	}

	return nil
}
