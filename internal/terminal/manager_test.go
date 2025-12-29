package terminal

import (
	"errors"
	"io"
	"os/exec"
	"sync"
	"testing"
)

type fakePty struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newFakePty() *fakePty {
	reader, writer := io.Pipe()
	return &fakePty{reader: reader, writer: writer}
}

func (p *fakePty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *fakePty) Write(data []byte) (int, error) {
	return p.writer.Write(data)
}

func (p *fakePty) Close() error {
	_ = p.reader.Close()
	return p.writer.Close()
}

func (p *fakePty) Resize(cols, rows uint16) error {
	return nil
}

type fakeFactory struct {
	mu   sync.Mutex
	ptys []*fakePty
}

func (f *fakeFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	pty := newFakePty()

	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
	f.mu.Unlock()

	return pty, nil, nil
}

func TestManagerLifecycle(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	first, err := manager.Create("build", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := manager.Create("run", "second")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected unique IDs")
	}

	if _, ok := manager.Get(first.ID); !ok {
		t.Fatalf("expected to get first session")
	}

	list := manager.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}

	if err := manager.Delete(first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if _, ok := manager.Get(first.ID); ok {
		t.Fatalf("expected first session to be deleted")
	}

	if err := manager.Delete("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
