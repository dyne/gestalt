package api

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type sseFrame struct {
	Event string
	Data  []byte
}

func TestServeSSEStreamSendsPayloadAndCloses(t *testing.T) {
	output := make(chan string, 1)
	handlerDone := make(chan struct{})

	srv := newSSETestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSSEStream(w, r, sseStreamConfig[string]{
			Output: output,
			BuildPayload: func(value string) (any, bool) {
				return map[string]string{"value": value}, true
			},
			HeartbeatInterval: time.Hour,
		})
		close(handlerDone)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get sse: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected content-type text/event-stream, got %q", got)
	}

	output <- "hello"

	reader := bufio.NewReader(resp.Body)
	frame, err := readSSEFrame(reader)
	if err != nil {
		t.Fatalf("read sse frame: %v", err)
	}
	if len(frame.Data) == 0 {
		frame, err = readSSEFrame(reader)
		if err != nil {
			t.Fatalf("read sse frame: %v", err)
		}
	}
	if len(frame.Data) == 0 {
		t.Fatalf("expected sse data frame")
	}

	var payload map[string]string
	if err := json.Unmarshal(frame.Data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["value"] != "hello" {
		t.Fatalf("unexpected payload: %v", payload)
	}

	resp.Body.Close()

	select {
	case <-handlerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("handler did not exit after close")
	}
}

func readSSEFrame(reader *bufio.Reader) (sseFrame, error) {
	var frame sseFrame
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return frame, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) > 0 {
				frame.Data = []byte(strings.Join(dataLines, "\n"))
			}
			return frame, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "retry:") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			frame.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
	}
}

func newSSETestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping sse test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}
