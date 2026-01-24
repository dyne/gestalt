package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServeWSStreamSendsPayloadAndCloses(t *testing.T) {
	output := make(chan string, 1)
	handlerDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWSStream(w, r, wsStreamConfig[string]{
			Output: output,
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
		})
		close(handlerDone)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	output <- "hello"

	var payload map[string]string
	if err := conn.ReadJSON(&payload); err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket: %v", err)
	}
	if payload["value"] != "hello" {
		_ = conn.Close()
		t.Fatalf("unexpected payload: %v", payload)
	}

	_ = conn.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}
