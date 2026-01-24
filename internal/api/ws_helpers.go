package api

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const wsReadBufferSize = 1024
const wsWriteBufferSize = 1024
const wsWriteTimeout = 10 * time.Second

type wsStreamConfig[T any] struct {
	AllowedOrigins []string
	Output         <-chan T
	BuildPayload   func(T) (any, bool)
	WriteTimeout   time.Duration
}

func requireWSToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if !validateToken(r, token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

	conn, err := upgradeWebSocket(w, r, config.AllowedOrigins)
	if err != nil {
		return
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
