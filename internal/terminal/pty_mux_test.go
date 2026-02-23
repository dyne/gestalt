package terminal

import (
	"io"
	"os/exec"
	"testing"
)

type recordingFactory struct {
	calls       int
	lastCommand string
	lastArgs    []string
	pty         Pty
}

func (f *recordingFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	f.calls++
	f.lastCommand = command
	f.lastArgs = append([]string(nil), args...)
	if f.pty == nil {
		f.pty = &noopPty{}
	}
	return f.pty, nil, nil
}

type noopPty struct{}

func (n *noopPty) Read(data []byte) (int, error)  { return 0, io.EOF }
func (n *noopPty) Write(data []byte) (int, error) { return len(data), nil }
func (n *noopPty) Close() error                   { return nil }
func (n *noopPty) Resize(cols, rows uint16) error { return nil }

func TestMuxPtyFactoryUsesTUI(t *testing.T) {
	tui := &recordingFactory{pty: &noopPty{}}
	stdio := &recordingFactory{pty: &noopPty{}}
	factory := NewMuxPtyFactory(tui, stdio, false)

	_, _, err := factory.Start("bash", "-c", "echo")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if tui.calls != 1 || stdio.calls != 0 {
		t.Fatalf("expected tui=1 stdio=0, got tui=%d stdio=%d", tui.calls, stdio.calls)
	}
}

func TestMuxPtyFactoryIgnoresSecondaryFactory(t *testing.T) {
	tui := &recordingFactory{pty: &noopPty{}}
	stdio := &recordingFactory{pty: &noopPty{}}
	factory := NewMuxPtyFactory(tui, stdio, false)

	pty, _, err := factory.Start("codex", "-c", "model=o3")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if tui.calls != 1 || stdio.calls != 0 {
		t.Fatalf("expected tui=1 stdio=0, got tui=%d stdio=%d", tui.calls, stdio.calls)
	}
	if _, ok := pty.(*noopPty); !ok {
		t.Fatalf("expected noop pty, got %T", pty)
	}
}
