package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

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
	if !validateToken(r, h.AuthToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if h.Logger == nil {
		http.Error(w, "logger unavailable", http.StatusInternalServerError)
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
		http.Error(w, "log stream unavailable", http.StatusInternalServerError)
		return
	}
	defer cancel()

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return isOriginAllowed(r, h.AllowedOrigins)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case entry, ok := <-output:
				if !ok {
					return
				}
				if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				if err := conn.WriteJSON(entry); err != nil {
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
