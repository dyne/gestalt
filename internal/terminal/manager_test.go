package terminal

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/skill"
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

type commandCaptureFactory struct {
	command string
	args    []string
}

func (f *commandCaptureFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	f.command = command
	f.args = append([]string(nil), args...)
	return newFakePty(), nil, nil
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

type scriptedPty struct {
	mu        sync.Mutex
	writes    [][]byte
	output    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newScriptedPty() *scriptedPty {
	return &scriptedPty{
		output: make(chan []byte, 8),
		closed: make(chan struct{}),
	}
}

func (p *scriptedPty) Read(data []byte) (int, error) {
	select {
	case chunk, ok := <-p.output:
		if !ok {
			return 0, io.EOF
		}
		n := copy(data, chunk)
		return n, nil
	case <-p.closed:
		return 0, io.EOF
	}
}

func (p *scriptedPty) Write(data []byte) (int, error) {
	p.mu.Lock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	p.mu.Unlock()
	return len(data), nil
}

func (p *scriptedPty) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		close(p.output)
	})
	return nil
}

func (p *scriptedPty) Resize(cols, rows uint16) error {
	return nil
}

func (p *scriptedPty) Emit(text string) {
	p.output <- []byte(text)
}

type scriptedFactory struct {
	pty *scriptedPty
}

func (f *scriptedFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	if f.pty == nil {
		f.pty = newScriptedPty()
	}
	return f.pty, nil, nil
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

func TestManagerCreateShellArgs(t *testing.T) {
	factory := &commandCaptureFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"copilot": {
				Name:     "Architect",
				Shell:    "copilot --allow-all-tools --disable-builtin-mcps",
				LLMType:  "copilot",
				LLMModel: "default",
			},
		},
	})

	if _, err := manager.Create("copilot", "run", "args"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if factory.command != "copilot" {
		t.Fatalf("expected command copilot, got %q", factory.command)
	}
	wantArgs := []string{"--allow-all-tools", "--disable-builtin-mcps"}
	if len(factory.args) != len(wantArgs) {
		t.Fatalf("expected args %v, got %v", wantArgs, factory.args)
	}
	for i, arg := range wantArgs {
		if factory.args[i] != arg {
			t.Fatalf("expected args %v, got %v", wantArgs, factory.args)
		}
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

func TestManagerAgentSingleInstance(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:  "Codex",
				Shell: "/bin/bash",
			},
		},
	})

	first, err := manager.Create("codex", "build", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	if _, ok := manager.GetSessionByAgent("Codex"); !ok {
		t.Fatalf("expected session for agent Codex")
	}

	if _, err := manager.Create("codex", "build", "second"); err == nil {
		t.Fatalf("expected duplicate agent error")
	} else {
		var dup *AgentAlreadyRunningError
		if !errors.As(err, &dup) {
			t.Fatalf("expected AgentAlreadyRunningError, got %v", err)
		}
		if dup.TerminalID != first.ID {
			t.Fatalf("expected terminal id %q, got %q", first.ID, dup.TerminalID)
		}
		if dup.AgentName != "Codex" {
			t.Fatalf("expected agent name Codex, got %q", dup.AgentName)
		}
	}

	if err := manager.Delete(first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if _, ok := manager.GetSessionByAgent("Codex"); ok {
		t.Fatalf("expected no session after delete")
	}

	if _, err := manager.Create("codex", "build", "third"); err != nil {
		t.Fatalf("create after delete: %v", err)
	}
}

func TestManagerGetAgentTerminal(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:  "Codex",
				Shell: "/bin/bash",
			},
		},
	})

	session, err := manager.Create("codex", "build", "first")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	id, ok := manager.GetAgentTerminal("Codex")
	if !ok {
		t.Fatalf("expected running terminal for Codex")
	}
	if id != session.ID {
		t.Fatalf("expected terminal id %q, got %q", session.ID, id)
	}
	if id, ok := manager.GetAgentTerminal("Missing"); ok || id != "" {
		t.Fatalf("expected missing agent to return empty and false")
	}
	if id, ok := manager.GetAgentTerminal(""); ok || id != "" {
		t.Fatalf("expected empty agent name to return empty and false")
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if id, ok := manager.GetAgentTerminal("Codex"); ok || id != "" {
		t.Fatalf("expected no terminal after delete")
	}
}

func TestManagerSkillsLoaded(t *testing.T) {
	entries := map[string]*skill.Skill{
		"git-workflows": {
			Name:        "git-workflows",
			Description: "Helpful git workflows",
			License:     "MIT",
			Path:        "config/skills/git-workflows",
		},
	}
	manager := NewManager(ManagerOptions{
		Skills: entries,
	})

	skillEntry, ok := manager.GetSkill("git-workflows")
	if !ok {
		t.Fatalf("expected git-workflows skill")
	}
	if skillEntry.Name != "git-workflows" {
		t.Fatalf("name mismatch: %q", skillEntry.Name)
	}

	infos := manager.ListSkills()
	if len(infos) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(infos))
	}
	if infos[0].Name != "git-workflows" {
		t.Fatalf("metadata name mismatch: %q", infos[0].Name)
	}
}

func TestManagerInjectsPrompt(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	firstPrompt := filepath.Join(promptsDir, "first.txt")
	if err := os.WriteFile(firstPrompt, []byte("echo hello\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	secondPrompt := filepath.Join(promptsDir, "second.txt")
	if err := os.WriteFile(secondPrompt, []byte("echo world\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	factory := &captureFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/bash",
				Prompts: agent.PromptList{"first", "second"},
				LLMType: "codex",
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

	deadline := time.Now().Add(6 * time.Second)
	expectedPrefix := "echo hello\necho world\n"
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) >= 2 {
			payload := ""
			for _, chunk := range writes {
				payload += string(chunk)
			}
			if len(payload) >= len(expectedPrefix) && !strings.HasPrefix(payload, expectedPrefix) {
				t.Fatalf("prompt payload mismatch: %q", payload)
			}
			if !strings.HasSuffix(payload, "\r\n") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for prompt write")
}

func TestManagerWritesSkillsMetadata(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	firstPrompt := filepath.Join(promptsDir, "first.txt")
	if err := os.WriteFile(firstPrompt, []byte("echo hello\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	factory := &captureFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Skills: map[string]*skill.Skill{
			"beta": {
				Name:        "beta",
				Description: "Second skill",
				Path:        "config/skills/beta",
				Content:     "# Beta Skill\nBeta body\n",
			},
			"alpha": {
				Name:        "alpha",
				Description: "First skill",
				Path:        "config/skills/alpha",
				Content:     "# Alpha Skill\nAlpha body\n",
			},
		},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/bash",
				Prompts: agent.PromptList{"first"},
				Skills:  []string{"beta", "alpha"},
				LLMType: "codex",
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

	deadline := time.Now().Add(6 * time.Second)
	payload := ""
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) > 0 {
			builder := strings.Builder{}
			for _, chunk := range writes {
				builder.Write(chunk)
			}
			payload = builder.String()
			if strings.HasSuffix(payload, "\r\n") {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if payload == "" {
		t.Fatalf("timed out waiting for prompt write")
	}
	// Skills metadata should be injected as XML
	if !strings.Contains(payload, "<available_skills>") {
		t.Fatalf("expected skills metadata in payload: %q", payload)
	}
	if !strings.Contains(payload, "<name>beta</name>") {
		t.Fatalf("expected beta skill in metadata: %q", payload)
	}
	if !strings.Contains(payload, "<name>alpha</name>") {
		t.Fatalf("expected alpha skill in metadata: %q", payload)
	}
	if !strings.Contains(payload, "<location>config/skills/beta/SKILL.md</location>") {
		t.Fatalf("expected beta skill location in metadata: %q", payload)
	}
	// Prompt should still be there after skills
	if !strings.Contains(payload, "echo hello\n") {
		t.Fatalf("prompt payload missing: %q", payload)
	}
	// Full skill content should NOT be in payload
	if strings.Contains(payload, "# Beta Skill") || strings.Contains(payload, "# Alpha Skill") {
		t.Fatalf("unexpected full skill content in payload: %q", payload)
	}
}

func TestManagerOnAirStringDelaysPrompt(t *testing.T) {
	oldTimeout := onAirTimeout
	onAirTimeout = 500 * time.Millisecond
	defer func() {
		onAirTimeout = oldTimeout
	}()

	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	promptPath := filepath.Join(promptsDir, "first.txt")
	if err := os.WriteFile(promptPath, []byte("echo hello\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	factory := &scriptedFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:        "Codex",
				Shell:       "/bin/bash",
				Prompts:     agent.PromptList{"first"},
				OnAirString: "READY",
				LLMType:     "codex",
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

	time.Sleep(100 * time.Millisecond)
	factory.pty.mu.Lock()
	pending := len(factory.pty.writes)
	factory.pty.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected no prompt writes before onair, got %d", pending)
	}

	factory.pty.Emit("ready\n")

	deadline := time.Now().Add(2 * time.Second)
	expectedPrefix := "echo hello\n"
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) > 0 {
			payload := ""
			for _, chunk := range writes {
				payload += string(chunk)
			}
			if len(payload) >= len(expectedPrefix) && !strings.HasPrefix(payload, expectedPrefix) {
				t.Fatalf("prompt payload mismatch: %q", payload)
			}
			if strings.HasSuffix(payload, "\r\n") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for prompt after onair")
}

func TestManagerOnAirTimeoutInjectsAnyway(t *testing.T) {
	oldTimeout := onAirTimeout
	onAirTimeout = 150 * time.Millisecond
	defer func() {
		onAirTimeout = oldTimeout
	}()

	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	promptPath := filepath.Join(promptsDir, "first.txt")
	if err := os.WriteFile(promptPath, []byte("echo hello\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	factory := &scriptedFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Logger:     logger,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:        "Codex",
				Shell:       "/bin/bash",
				Prompts:     agent.PromptList{"first"},
				OnAirString: "READY",
				LLMType:     "codex",
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

	deadline := time.Now().Add(2 * time.Second)
	wrotePrompt := false
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) > 0 {
			wrotePrompt = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !wrotePrompt {
		t.Fatalf("expected prompt writes after onair timeout")
	}

	entries := buffer.List()
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelError && entry.Message == "agent onair string not found" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error log for onair timeout")
	}
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

func TestManagerCreateUnknownAgentLogsWarning(t *testing.T) {
	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
	manager := NewManager(ManagerOptions{
		Logger: logger,
	})

	if _, err := manager.Create("missing", "role", "title"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Message != "agent not found or invalid" {
		t.Fatalf("expected agent not found or invalid log, got %q", entry.Message)
	}
	if entry.Context["agent_id"] != "missing" {
		t.Fatalf("expected agent_id missing, got %v", entry.Context)
	}
}
