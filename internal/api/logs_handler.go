package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"

	"github.com/gorilla/websocket"
)

type LogsHandler struct {
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

type logFilterMessage struct {
	Level string `json:"level"`
}

type levelFilter struct {
	mu    sync.RWMutex
	level logging.Level
}

func (f *levelFilter) Get() logging.Level {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.level
}

func (f *levelFilter) Set(level logging.Level) {
	f.mu.Lock()
	f.level = level
	f.mu.Unlock()
}

func (h *LogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireWSToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	filter := &levelFilter{}
	if rawLevel := r.URL.Query().Get("level"); rawLevel != "" {
		if level, ok := logging.ParseLevel(rawLevel); ok {
			filter.Set(level)
		}
	}

	hub := otel.ActiveLogHub()
	if hub == nil {
		writeWSError(w, r, nil, h.Logger, wsError{
			Status:       http.StatusServiceUnavailable,
			Message:      "log stream unavailable",
			SendEnvelope: true,
		})
		return
	}

	output, cancel := hub.Subscribe()
	if output == nil {
		writeWSError(w, r, nil, h.Logger, wsError{
			Status:       http.StatusServiceUnavailable,
			Message:      "log stream unavailable",
			SendEnvelope: true,
		})
		return
	}

	conn, err := upgradeWebSocket(w, r, h.AllowedOrigins)
	if err != nil {
		cancel()
		logWSError(h.Logger, r, wsError{
			Status:  http.StatusBadRequest,
			Message: "websocket upgrade failed",
			Err:     err,
		})
		return
	}
	defer conn.Close()

	spanCtx, span := startWebSocketSpan(r, "/ws/logs")
	defer span.End()
	r = r.WithContext(spanCtx)

	snapshot := hub.SnapshotSince(time.Time{})
	writer, err := startWSWriteLoop(w, r, wsStreamConfig[map[string]any]{
		Conn:           conn,
		AllowedOrigins: h.AllowedOrigins,
		Output:         output,
		Logger:         h.Logger,
		PreWrite: func(conn *websocket.Conn) error {
			return writeLogSnapshot(conn, snapshot, filter.Get())
		},
		BuildPayload: func(entry map[string]any) (any, bool) {
			if entry == nil {
				return nil, false
			}
			minLevel := filter.Get()
			if minLevel != "" && !logging.LevelAtLeast(otelLogLevel(entry), minLevel) {
				return nil, false
			}
			return entry, true
		},
	})
	if err != nil {
		cancel()
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "log stream unavailable",
			Err:          err,
			SendEnvelope: true,
		})
		return
	}
	defer cancel()
	defer writer.Stop()

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var payload logFilterMessage
		if err := json.Unmarshal(msg, &payload); err != nil {
			continue
		}
		level, ok := logging.ParseLevel(payload.Level)
		if !ok {
			filter.Set("")
			continue
		}
		filter.Set(level)
	}
}

func writeLogSnapshot(conn *websocket.Conn, entries []map[string]any, minLevel logging.Level) error {
	if conn == nil || len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if minLevel != "" && !logging.LevelAtLeast(otelLogLevel(entry), minLevel) {
			continue
		}
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
			return err
		}
		if err := writeJSONPayload(conn, annotateReplay(entry)); err != nil {
			return err
		}
	}
	return nil
}

func annotateReplay(entry map[string]any) map[string]any {
	cloned := make(map[string]any, len(entry))
	for key, value := range entry {
		cloned[key] = value
	}
	attributes := asSlice(entry["attributes"])
	updated := make([]any, 0, len(attributes)+1)
	if len(attributes) > 0 {
		updated = append(updated, attributes...)
	}
	updated = append(updated, map[string]any{
		"key": "gestalt.replay_window",
		"value": map[string]any{
			"stringValue": logReplayWindowValue(),
		},
	})
	cloned["attributes"] = updated
	return cloned
}

func logReplayWindowValue() string {
	if otel.DefaultLogHubMaxRecords <= 0 {
		return "max-entries"
	}
	return fmt.Sprintf("max-entries:%d", otel.DefaultLogHubMaxRecords)
}

func otelLogLevel(record map[string]any) logging.Level {
	if text, ok := extractString(record, "severityText", "severity_text", "severity", "level"); ok {
		if level, parsed := logging.ParseLevel(text); parsed {
			return level
		}
	}
	if value, ok := extractNumber(record, "severityNumber", "severity_number"); ok {
		return severityFromNumber(value)
	}
	return logging.LevelInfo
}

func severityFromNumber(value float64) logging.Level {
	switch {
	case value >= 17:
		return logging.LevelError
	case value >= 13:
		return logging.LevelWarning
	case value >= 9:
		return logging.LevelInfo
	default:
		return logging.LevelDebug
	}
}
