package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/api"
	"gestalt/internal/terminal"
)

type capturePty struct {
	mu     sync.Mutex
	writes [][]byte
	closed chan struct{}
}

func newCapturePty() *capturePty {
	return &capturePty{closed: make(chan struct{})}
}

func (p *capturePty) Read(data []byte) (int, error) {
	<-p.closed
	return 0, io.EOF
}

func (p *capturePty) Write(data []byte) (int, error) {
	p.mu.Lock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	p.mu.Unlock()
	return len(data), nil
}

func (p *capturePty) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func (p *capturePty) Resize(cols, rows uint16) error {
	return nil
}

type captureFactory struct {
	pty *capturePty
}

func (f *captureFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	if f.pty == nil {
		f.pty = newCapturePty()
	}
	return f.pty, nil, nil
}

func TestGestaltSendEndToEnd(t *testing.T) {
	factory := &captureFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:  "Codex",
				Shell: "/bin/sh",
			},
		},
	})

	session, err := manager.Create("codex", "shell", "Codex")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, "secret", api.StatusConfig{}, "", false, "", nil, nil, nil, nil)

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})

	withAgentCacheDisabled(t, func() {
		withMockClient(t, transport, func() {
			t.Setenv("GESTALT_URL", "http://example.invalid")
			t.Setenv("GESTALT_TOKEN", "secret")
			var stderr bytes.Buffer

			code := runWithSender([]string{"Codex"}, strings.NewReader("ping"), &stderr, sendAgentInput)
			if code != 0 {
				t.Fatalf("expected exit code 0, got %d: %s", code, stderr.String())
			}

			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) {
				factory.pty.mu.Lock()
				if len(factory.pty.writes) > 0 {
					combined := bytes.Join(factory.pty.writes, nil)
					factory.pty.mu.Unlock()
					if string(combined) != "ping" {
						t.Fatalf("expected payload %q, got %q", "ping", string(combined))
					}
					return
				}
				factory.pty.mu.Unlock()
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("timed out waiting for PTY write")
		})
	})
}
