package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/watcher"

	"github.com/gorilla/websocket"
)

type eventWireMessage struct {
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

func TestEventsWebSocketFileChange(t *testing.T) {
	fsWatcher, err := watcher.NewWithOptions(watcher.Options{Debounce: 20 * time.Millisecond})
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	file, err := os.CreateTemp("", "gestalt-events-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	if _, err := watcher.WatchFile(bus, fsWatcher, path); err != nil {
		t.Fatalf("watch file: %v", err)
	}

	server := startEventServer(t, bus)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	connA, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket A: %v", err)
	}
	defer connA.Close()
	connB, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket B: %v", err)
	}
	defer connB.Close()

	if err := os.WriteFile(path, []byte("update"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	if err := connA.SetReadDeadline(deadline); err != nil {
		t.Fatalf("set read deadline A: %v", err)
	}
	var msgA eventWireMessage
	if err := connA.ReadJSON(&msgA); err != nil {
		t.Fatalf("read websocket A: %v", err)
	}
	if msgA.Type != watcher.EventTypeFileChanged {
		t.Fatalf("expected file_changed, got %q", msgA.Type)
	}
	if msgA.Path != path {
		t.Fatalf("expected path %q, got %q", path, msgA.Path)
	}

	if err := connB.SetReadDeadline(deadline); err != nil {
		t.Fatalf("set read deadline B: %v", err)
	}
	var msgB eventWireMessage
	if err := connB.ReadJSON(&msgB); err != nil {
		t.Fatalf("read websocket B: %v", err)
	}
	if msgB.Type != watcher.EventTypeFileChanged {
		t.Fatalf("expected file_changed, got %q", msgB.Type)
	}
	if msgB.Path != path {
		t.Fatalf("expected path %q, got %q", path, msgB.Path)
	}
}

func TestEventsWebSocketReconnect(t *testing.T) {
	fsWatcher, err := watcher.NewWithOptions(watcher.Options{Debounce: 20 * time.Millisecond})
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	file, err := os.CreateTemp("", "gestalt-events-reconnect-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	if _, err := watcher.WatchFile(bus, fsWatcher, path); err != nil {
		t.Fatalf("watch file: %v", err)
	}

	server := startEventServer(t, bus)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	conn.Close()

	conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("redial websocket: %v", err)
	}
	defer conn.Close()

	if err := os.WriteFile(path, []byte("update"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var payload eventWireMessage
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != watcher.EventTypeFileChanged {
		t.Fatalf("expected file_changed, got %q", payload.Type)
	}
}

func TestEventsWebSocketGitBranchChange(t *testing.T) {
	fsWatcher, err := watcher.NewWithOptions(watcher.Options{Debounce: 20 * time.Millisecond})
	if err != nil {
		t.Skipf("skipping watcher test (fsnotify unavailable): %v", err)
	}
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}

	if _, err := watcher.StartGitWatcher(bus, fsWatcher, workDir); err != nil {
		t.Fatalf("start git watcher: %v", err)
	}

	server := startEventServer(t, bus)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := os.WriteFile(headPath, []byte("ref: refs/heads/feature\n"), 0o644); err != nil {
		t.Fatalf("update head: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var payload eventWireMessage
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if payload.Type != watcher.EventTypeGitBranchChanged {
		t.Fatalf("expected git_branch_changed, got %q", payload.Type)
	}
	if payload.Path != "feature" {
		t.Fatalf("expected branch feature, got %q", payload.Path)
	}
}

func TestEventsWebSocketSubscriberCleanup(t *testing.T) {
	bus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{Name: "watcher_events"})
	defer bus.Close()

	server := startEventServer(t, bus)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return bus.SubscriberCount() > 0
	})

	conn.Close()

	waitForCondition(t, 200*time.Millisecond, func() bool {
		return bus.SubscriberCount() == 0
	})
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func startEventServer(t *testing.T, bus *event.Bus[watcher.Event]) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &EventsHandler{Bus: bus}},
	}
	server.Start()
	return server
}
