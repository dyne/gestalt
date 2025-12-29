package terminal

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
)

type fakePty struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	err    error
}

func newFakePty() *fakePty {
	return newFakePtyWithErr(nil)
}

func newFakePtyWithErr(err error) *fakePty {
	reader, writer := io.Pipe()
	return &fakePty{reader: reader, writer: writer, err: err}
}

func (p *fakePty) Read(data []byte) (int, error) {
	return p.reader.Read(data)
}

func (p *fakePty) Write(data []byte) (int, error) {
	return p.writer.Write(data)
}

func (p *fakePty) Close() error {
	_ = p.reader.Close()
	_ = p.writer.Close()
	return p.err
}

func (p *fakePty) Resize(cols, rows uint16) error {
	return nil
}

type fakeFactory struct {
	mu     sync.Mutex
	ptys   []*fakePty
	newPty func() *fakePty
}

func (f *fakeFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	var pty *fakePty
	if f.newPty != nil {
		pty = f.newPty()
	} else {
		pty = newFakePty()
	}

	f.mu.Lock()
	f.ptys = append(f.ptys, pty)
	f.mu.Unlock()

	return pty, nil, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

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

func (f *captureFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	if f.pty == nil {
		f.pty = newCapturePty()
	}
	return f.pty, nil, nil
}

func TestManagerLifecycle(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	first, err := manager.Create("", "build", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := manager.Create("", "run", "second")
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

func TestManagerUsesClock(t *testing.T) {
	factory := &fakeFactory{}
	now := time.Date(2024, 2, 10, 8, 30, 0, 0, time.FixedZone("test", 2*60*60))
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Clock:      fixedClock{now: now},
	})

	session, err := manager.Create("", "build", "clocked")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if !session.CreatedAt.Equal(now.UTC()) {
		t.Fatalf("expected CreatedAt %v, got %v", now.UTC(), session.CreatedAt)
	}
}

func TestManagerGetAgent(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/bash",
				LLMType: "codex",
			},
		},
	})

	profile, ok := manager.GetAgent("codex")
	if !ok {
		t.Fatalf("expected codex agent")
	}
	if profile.Name != "Codex" {
		t.Fatalf("name mismatch: %q", profile.Name)
	}
	if _, ok := manager.GetAgent("missing"); ok {
		t.Fatalf("expected missing agent to be false")
	}
}

func TestManagerInjectsPrompt(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.sh")
	if err := os.WriteFile(promptPath, []byte("echo hello"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	factory := &captureFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:       "Codex",
				Shell:      "/bin/bash",
				PromptFile: promptPath,
				LLMType:    "codex",
			},
		},
	})

	session, err := manager.Create("codex", "build", "ignored")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) > 0 {
			got := string(writes[0])
			if got != "echo hello\n" {
				t.Fatalf("prompt mismatch: %q", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for prompt write")
}

func TestManagerDeleteIgnoresCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	factory := &fakeFactory{
		newPty: func() *fakePty {
			return newFakePtyWithErr(closeErr)
		},
	}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	session, err := manager.Create("", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
