package desktop

import (
	"context"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"gestalt/internal/terminal"
)

type stubPty struct{}

func (p *stubPty) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (p *stubPty) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *stubPty) Close() error {
	return nil
}

func (p *stubPty) Resize(_, _ uint16) error {
	return nil
}

type stubPtyFactory struct{}

func (f *stubPtyFactory) Start(_ string, _ ...string) (terminal.Pty, *exec.Cmd, error) {
	return &stubPty{}, nil, nil
}

func TestShutdownClosesSessions(t *testing.T) {
	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &stubPtyFactory{},
	})
	if _, err := manager.Create("", "run", "first"); err != nil {
		t.Fatalf("create first session: %v", err)
	}
	if _, err := manager.Create("", "run", "second"); err != nil {
		t.Fatalf("create second session: %v", err)
	}

	app := NewApp("http://127.0.0.1:0", manager, &http.Server{}, nil)
	app.Shutdown(context.Background())

	if sessions := manager.List(); len(sessions) != 0 {
		t.Fatalf("expected no sessions after shutdown, got %d", len(sessions))
	}
}
