package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/runner/launchspec"

	"github.com/gorilla/websocket"
)

type fakeBridgeClient struct {
	mu           sync.Mutex
	pipeTarget   string
	pipeCommand  string
	capture      []byte
	loadBuffers  [][]byte
	pasteTargets []string
}

func (f *fakeBridgeClient) PipePane(target, command string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pipeTarget = target
	f.pipeCommand = command
	return nil
}

func (f *fakeBridgeClient) CapturePane(target string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.capture...), nil
}

func (f *fakeBridgeClient) ResizePane(target string, cols, rows uint16) error {
	return nil
}

func (f *fakeBridgeClient) LoadBuffer(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loadBuffers = append(f.loadBuffers, append([]byte(nil), data...))
	return nil
}

func (f *fakeBridgeClient) PasteBuffer(target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pasteTargets = append(f.pasteTargets, target)
	return nil
}

func (f *fakeBridgeClient) KillSession(name string) error {
	return nil
}

func TestRunnerWebSocketURLPreservesSessionID(t *testing.T) {
	got, err := runnerWebSocketURL("http://localhost:57417", "Fixer 1")
	if err != nil {
		t.Fatalf("runnerWebSocketURL error: %v", err)
	}
	want := "ws://localhost:57417/ws/runner/session/Fixer%201"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRunnerBridgeForwardsIO(t *testing.T) {
	fake := &fakeBridgeClient{capture: []byte("snapshot")}
	originalFactory := tmuxBridgeFactory
	tmuxBridgeFactory = func() tmuxBridgeClient { return fake }
	t.Cleanup(func() { tmuxBridgeFactory = originalFactory })

	originalTail := tailFileFunc
	tailFileFunc = func(ctx context.Context, path string, onChunk func([]byte) error) error {
		_ = onChunk([]byte("tail"))
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { tailFileFunc = originalTail })

	upgrader := websocket.Upgrader{}
	var received [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage() // hello
		for len(received) < 2 {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				received = append(received, msg)
			}
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("input"))
		_ = conn.Close()
	}))
	defer server.Close()

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex"},
	}
	err := runRunnerBridge(context.Background(), launch, server.URL, "")
	if err == nil {
		t.Fatalf("expected bridge to exit with error after close")
	}

	joined := string(received[0]) + string(received[1])
	if !strings.Contains(joined, "snapshot") || !strings.Contains(joined, "tail") {
		t.Fatalf("expected snapshot and tail output, got %q", joined)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	expectedTarget := tmuxSessionName(launch.SessionID)
	if fake.pipeTarget != expectedTarget {
		t.Fatalf("expected pipe target %q, got %q", expectedTarget, fake.pipeTarget)
	}
	if len(fake.loadBuffers) == 0 || string(fake.loadBuffers[0]) != "input" {
		t.Fatalf("expected input to be loaded, got %#v", fake.loadBuffers)
	}
	if len(fake.pasteTargets) == 0 || fake.pasteTargets[0] == "" {
		t.Fatalf("expected paste target, got %#v", fake.pasteTargets)
	}
}
