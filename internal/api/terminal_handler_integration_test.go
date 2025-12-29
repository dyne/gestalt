package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/terminal"

	"github.com/gorilla/websocket"
)

type testPty struct {
	reader  *io.PipeReader
	writer  *io.PipeWriter
	writeCh chan []byte
}

func newTestPty() *testPty {
	reader, writer := io.Pipe()
	return &testPty{
		reader:  reader,
		writer:  writer,
		writeCh: make(chan []byte, 4),
	}
}

func (p *testPty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *testPty) Write(data []byte) (int, error) {
	copied := append([]byte(nil), data...)
	select {
	case p.writeCh <- copied:
	default:
	}
	return len(data), nil
}

func (p *testPty) Close() error {
	_ = p.reader.Close()
	return p.writer.Close()
}

func (p *testPty) Resize(cols, rows uint16) error {
	return nil
}

func (p *testPty) emitOutput(data []byte) error {
	_, err := p.writer.Write(data)
	return err
}

func (p *testPty) waitForWrite(expected []byte, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case got := <-p.writeCh:
		return bytes.Equal(got, expected)
	case <-timer.C:
		return false
	}
}

type testFactory struct {
	mu   sync.Mutex
	ptys []*testPty
}

func (f *testFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	pty := newTestPty()
	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
	f.mu.Unlock()
	return pty, nil, nil
}

func TestTerminalWebSocketBridge(t *testing.T) {
	factory := &testFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create("test", "ws")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	factory.mu.Lock()
	pty := factory.ptys[0]
	factory.mu.Unlock()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket test (listener unavailable): %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: &TerminalHandler{Manager: manager}},
	}
	server.Start()
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/terminal/" + session.ID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := pty.emitOutput([]byte("hello\n")); err != nil {
		t.Fatalf("emit output: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}
	if !bytes.Contains(msg, []byte("hello")) {
		t.Fatalf("expected output to contain hello, got %q", string(msg))
	}

	payload := []byte("ls\n")
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("write websocket: %v", err)
	}

	if !pty.waitForWrite(payload, 500*time.Millisecond) {
		t.Fatalf("expected PTY to receive %q", string(payload))
	}
}
