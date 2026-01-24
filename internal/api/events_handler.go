package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/watcher"

	"github.com/gorilla/websocket"
)

const eventsPerMinuteLimit = 100

type EventsHandler struct {
	Bus            *event.Bus[watcher.Event]
	Logger         *logging.Logger
	AuthToken      string
	AllowedOrigins []string
}

type eventSubscribeMessage struct {
	Subscribe []string `json:"subscribe"`
}

type eventPayload struct {
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

type eventFilter struct {
	mutex sync.RWMutex
	types map[string]struct{}
}

func newEventFilter(allowed map[string]struct{}) *eventFilter {
	types := make(map[string]struct{}, len(allowed))
	for eventType := range allowed {
		types[eventType] = struct{}{}
	}
	return &eventFilter{types: types}
}

func (filter *eventFilter) Allows(eventType string) bool {
	if filter == nil {
		return true
	}
	filter.mutex.RLock()
	defer filter.mutex.RUnlock()
	if len(filter.types) == 0 {
		return false
	}
	_, ok := filter.types[eventType]
	return ok
}

func (filter *eventFilter) Set(subscriptions []string, allowed map[string]struct{}) {
	if filter == nil {
		return
	}
	types := make(map[string]struct{})
	for _, eventType := range subscriptions {
		if _, ok := allowed[eventType]; ok {
			types[eventType] = struct{}{}
		}
	}
	filter.mutex.Lock()
	filter.types = types
	filter.mutex.Unlock()
}

type rateLimiter struct {
	mutex       sync.Mutex
	count       int
	windowStart time.Time
}

func (limiter *rateLimiter) Allow(now time.Time) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	if limiter.windowStart.IsZero() || now.Sub(limiter.windowStart) >= time.Minute {
		limiter.windowStart = now
		limiter.count = 0
	}
	if limiter.count >= eventsPerMinuteLimit {
		return false
	}
	limiter.count++
	return true
}

func (h *EventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireWSToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	allowed := map[string]struct{}{
		watcher.EventTypeFileChanged:      {},
		watcher.EventTypeGitBranchChanged: {},
		watcher.EventTypeWatchError:       {},
	}

	filter := newEventFilter(allowed)
	limiter := &rateLimiter{}

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

	if h.Bus == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "event bus unavailable",
			SendEnvelope: true,
		})
		return
	}

	events, cancel := h.Bus.SubscribeFiltered(func(event watcher.Event) bool {
		_, ok := allowed[event.Type]
		return ok
	})
	if events == nil {
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "event stream unavailable",
			SendEnvelope: true,
		})
		return
	}

	writer, err := startWSWriteLoop(w, r, wsStreamConfig[watcher.Event]{
		Conn:           conn,
		AllowedOrigins: h.AllowedOrigins,
		Output:         events,
		Logger:         h.Logger,
		BuildPayload: func(event watcher.Event) (any, bool) {
			if !filter.Allows(event.Type) {
				return nil, false
			}
			if !limiter.Allow(time.Now()) {
				return nil, false
			}
			payload := eventPayload{
				Type:      event.Type,
				Path:      event.Path,
				Timestamp: event.Timestamp,
			}
			if payload.Timestamp.IsZero() {
				payload.Timestamp = time.Now().UTC()
			}
			return payload, true
		},
	})
	if err != nil {
		cancel()
		writeWSError(w, r, conn, h.Logger, wsError{
			Status:       http.StatusInternalServerError,
			Message:      "event stream unavailable",
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
		var payload eventSubscribeMessage
		if err := json.Unmarshal(msg, &payload); err != nil {
			continue
		}
		filter.Set(payload.Subscribe, allowed)
	}
}
