package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"gestalt/internal/logging"

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

	if h.Logger == nil {
		writeWSError(w, r, nil, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "logger unavailable",
			SendEnvelope: true,
		})
		return
	}

	filter := &levelFilter{}
	if rawLevel := r.URL.Query().Get("level"); rawLevel != "" {
		if level, ok := logging.ParseLevel(rawLevel); ok {
			filter.Set(level)
		}
	}

	output, cancel := h.Logger.SubscribeFiltered(func(entry logging.LogEntry) bool {
		minLevel := filter.Get()
		if minLevel == "" {
			return true
		}
		return logging.LevelAtLeast(entry.Level, minLevel)
	})
	if output == nil {
		writeWSError(w, r, nil, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "log stream unavailable",
			SendEnvelope: true,
		})
		return
	}

	writer, err := startWSWriteLoop(w, r, wsStreamConfig[logging.LogEntry]{
		AllowedOrigins: h.AllowedOrigins,
		Output:         output,
		Logger:         h.Logger,
	})
	if err != nil {
		cancel()
		logWSError(h.Logger, r, wsError{
			Status:  http.StatusBadRequest,
			Message: "websocket upgrade failed",
			Err:     err,
		})
		return
	}
	defer cancel()
	defer writer.Stop()

	conn := writer.Conn
	defer conn.Close()

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
