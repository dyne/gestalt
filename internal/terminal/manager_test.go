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
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/ports"
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

// timingPty records write times and can fail after a number of writes.
type timingPty struct {
	mu        sync.Mutex
	writes    [][]byte
	times     []time.Time
	failAfter int // -1 means never fail
	closed    chan struct{}
}

func newTimingPty(failAfter int) *timingPty {
	return &timingPty{failAfter: failAfter, closed: make(chan struct{})}
}

func (p *timingPty) Read(data []byte) (int, error) {
	<-p.closed
	return 0, io.EOF
}

func (p *timingPty) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	p.times = append(p.times, time.Now())
	if p.failAfter >= 0 && len(p.writes) > p.failAfter {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func (p *timingPty) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func TestRenderOutputTailFiltersOutput(t *testing.T) {
	lines := []string{
		"hello\x1b[31mred\x1b[0m",
		"-----",
	}
	output := renderOutputTail(nil, lines, 12, 2000)
	if strings.Contains(output, "\x1b") {
		t.Fatalf("expected ANSI sequences stripped, got %q", output)
	}
	if strings.Contains(output, "-----") {
		t.Fatalf("expected repeated chars collapsed, got %q", output)
	}
	if !strings.Contains(output, "red") {
		t.Fatalf("expected content preserved, got %q", output)
	}
}

func (p *timingPty) Resize(cols, rows uint16) error { return nil }

type timingFactory struct {
	pty       *timingPty
	failAfter int
}

func (f *timingFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	if f.pty == nil {
		f.pty = newTimingPty(f.failAfter)
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
				CLIType:  "copilot",
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

func TestManagerCreateWithCLIConfigUsesGeneratedCommand(t *testing.T) {
	factory := &commandCaptureFactory{}
	profile := agent.Agent{
		Name:    "Codex",
		CLIType: "codex",
		CLIConfig: map[string]interface{}{
			"model": "o3",
		},
	}
	profile.ConfigHash = agent.ComputeConfigHash(&profile)
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": profile,
		},
	})

	session, err := manager.Create("codex", "run", "cfg")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if factory.command != "codex" {
		t.Fatalf("expected command codex, got %q", factory.command)
	}
	wantArgs := []string{"-c", "model=o3"}
	if len(factory.args) < len(wantArgs) {
		t.Fatalf("expected args to include %v, got %v", wantArgs, factory.args)
	}
	for i, arg := range wantArgs {
		if factory.args[i] != arg {
			t.Fatalf("expected args %v, got %v", wantArgs, factory.args)
		}
	}
	notifyArg := ""
	for _, arg := range factory.args {
		if strings.Contains(arg, "notify=") {
			notifyArg = arg
			break
		}
	}
	if notifyArg == "" {
		t.Fatalf("expected notify flag in args, got %v", factory.args)
	}
	if !strings.Contains(notifyArg, "gestalt-notify") {
		t.Fatalf("expected notify command to include gestalt-notify, got %q", notifyArg)
	}
	if session.ConfigHash != profile.ConfigHash {
		t.Fatalf("expected config hash %q, got %q", profile.ConfigHash, session.ConfigHash)
	}
	if !strings.Contains(session.Command, "notify=") {
		t.Fatalf("expected notify in command, got %q", session.Command)
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
				CLIType: "codex",
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

func TestManagerAgentMultipleInstances(t *testing.T) {
	factory := &fakeFactory{}
	nonSingleton := false
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"architect": {
				Name:      "Architect",
				Shell:     "/bin/bash",
				Singleton: &nonSingleton,
			},
		},
	})

	first, err := manager.Create("architect", "build", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if first.ID != "architect-1" {
		t.Fatalf("expected id architect-1, got %q", first.ID)
	}

	second, err := manager.Create("architect", "build", "second")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.ID != "architect-2" {
		t.Fatalf("expected id architect-2, got %q", second.ID)
	}

	third, err := manager.Create("architect", "build", "third")
	if err != nil {
		t.Fatalf("create third: %v", err)
	}
	if third.ID != "architect-3" {
		t.Fatalf("expected id architect-3, got %q", third.ID)
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
				CLIType: "codex",
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
	promptDone := false
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
			promptDone = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !promptDone {
		t.Fatalf("timed out waiting for prompt write")
	}
	if len(session.PromptFiles) != 2 {
		t.Fatalf("expected 2 prompt files, got %d", len(session.PromptFiles))
	}
	if session.PromptFiles[0] != "first.txt" || session.PromptFiles[1] != "second.txt" {
		t.Fatalf("unexpected prompt files: %#v", session.PromptFiles)
	}
}

func TestManagerInjectsTemplatePrompt(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	mainPrompt := filepath.Join(promptsDir, "main.tmpl")
	if err := os.WriteFile(mainPrompt, []byte("echo start\n{{include fragment.txt}}\necho end\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	fragmentPrompt := filepath.Join(promptsDir, "fragment.txt")
	if err := os.WriteFile(fragmentPrompt, []byte("echo mid\n"), 0644); err != nil {
		t.Fatalf("write fragment: %v", err)
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
	factory := &captureFactory{}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Logger:     logger,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/bash",
				Prompts: agent.PromptList{"main"},
				CLIType: "codex",
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
	expectedPrefix := "echo start\necho mid\necho end\n"
	promptDone := false
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
			if !strings.HasSuffix(payload, "\r\n") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			promptDone = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !promptDone {
		t.Fatalf("timed out waiting for prompt write")
	}
	if len(session.PromptFiles) != 2 {
		t.Fatalf("expected 2 prompt files, got %d", len(session.PromptFiles))
	}
	if session.PromptFiles[0] != "main.tmpl" || session.PromptFiles[1] != "fragment.txt" {
		t.Fatalf("unexpected prompt files: %#v", session.PromptFiles)
	}

	entries := buffer.List()
	found := false
	for _, entry := range entries {
		if entry.Level != logging.LevelInfo || entry.Message != "agent prompt rendered" {
			continue
		}
		if entry.Context["agent_name"] != "Codex" {
			t.Fatalf("unexpected agent_name: %v", entry.Context["agent_name"])
		}
		if entry.Context["prompt_files"] != "main.tmpl, fragment.txt" {
			t.Fatalf("unexpected prompt_files: %v", entry.Context["prompt_files"])
		}
		if entry.Context["file_count"] != "2" {
			t.Fatalf("unexpected file_count: %v", entry.Context["file_count"])
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected prompt rendered log entry")
	}
}

func TestManagerInjectsPortDirectivePrompt(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	portPrompt := filepath.Join(promptsDir, "port.tmpl")
	if err := os.WriteFile(portPrompt, []byte("echo start\n{{port backend}}\necho end\n"), 0644); err != nil {
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
	factory := &captureFactory{}
	registry := ports.NewPortRegistry()
	registry.Set("backend", 18080)

	manager := NewManager(ManagerOptions{
		PtyFactory:   factory,
		Logger:       logger,
		PortResolver: registry,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Shell:   "/bin/bash",
				Prompts: agent.PromptList{"port"},
				CLIType: "codex",
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
	expectedPrefix := "echo start\n18080\necho end\n"
	promptDone := false
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
			if !strings.HasSuffix(payload, "\r\n") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			promptDone = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !promptDone {
		t.Fatalf("timed out waiting for prompt write")
	}
	if len(session.PromptFiles) != 1 || session.PromptFiles[0] != "port.tmpl" {
		t.Fatalf("unexpected prompt files: %#v", session.PromptFiles)
	}
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
				CLIType: "codex",
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
	// Skills metadata should NOT be written to terminal output
	if strings.Contains(payload, "<available_skills>") {
		t.Fatalf("unexpected skills metadata in payload: %q", payload)
	}
	// Prompt should still be present
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
				CLIType:     "codex",
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

func TestPromptInjectionTiming_WithMockAgent(t *testing.T) {
	// Prepare a long prompt to force multiple chunks
	root := t.TempDir()
	promptsDir := filepath.Join(root, "config", "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	// Create ~512 bytes to exceed multiple 64-byte chunks
	var b strings.Builder
	for i := 0; i < 16; i++ { // 16*32 = 512
		b.WriteString("0123456789ABCDEFGHIJKLMNOPQRSTUV\n") // 32 bytes incl. \n
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "long.txt"), []byte(b.String()), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	buffer := logging.NewLogBuffer(100)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelDebug, nil)

	factory := &timingFactory{failAfter: -1} // never fail; measure timing
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		Logger:     logger,
		Agents: map[string]agent.Agent{
			"mock": {
				Name:    "Mock",
				Shell:   "/bin/bash",
				Prompts: agent.PromptList{"long"},
				CLIType: "mock",
			},
		},
	})

	session, err := manager.Create("mock", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() { _ = manager.Delete(session.ID) }()

	// Wait for multiple chunk writes (promptDelay is 3s)
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		count := len(factory.pty.writes)
		factory.pty.mu.Unlock()
		if count >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	factory.pty.mu.Lock()
	writes := append([][]byte(nil), factory.pty.writes...)
	times := append([]time.Time(nil), factory.pty.times...)
	factory.pty.mu.Unlock()
	if len(writes) < 2 {
		t.Fatalf("expected multiple chunk writes, got %d", len(writes))
	}
	// Verify timing gaps roughly respect chunk delay (>= ~20ms between first two)
	if len(times) >= 2 {
		delta := times[1].Sub(times[0])
		if delta < 20*time.Millisecond {
			t.Fatalf("expected inter-chunk delay >=20ms, got %v", delta)
		}
	}
	// Ensure no skills XML is printed
	payload := strings.Builder{}
	for _, w := range writes {
		payload.Write(w)
	}
	if strings.Contains(payload.String(), "<available_skills>") {
		t.Fatalf("unexpected skills metadata in payload: %q", payload.String())
	}
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
				CLIType:     "codex",
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

func TestManagerAgentEvents(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex"},
		},
	})

	events, cancel := manager.AgentBus().Subscribe()
	defer cancel()

	session, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create agent session: %v", err)
	}

	started := receiveAgentEvent(t, events)
	if started.Type() != "agent_started" {
		t.Fatalf("expected agent_started, got %q", started.Type())
	}
	if started.AgentID != "codex" {
		t.Fatalf("expected agent_id codex, got %q", started.AgentID)
	}
	if started.AgentName != "Codex" {
		t.Fatalf("expected agent_name Codex, got %q", started.AgentName)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete agent session: %v", err)
	}
	stopped := receiveAgentEvent(t, events)
	if stopped.Type() != "agent_stopped" {
		t.Fatalf("expected agent_stopped, got %q", stopped.Type())
	}
	if stopped.AgentID != "codex" {
		t.Fatalf("expected agent_id codex, got %q", stopped.AgentID)
	}
}

func receiveAgentEvent(t *testing.T, ch <-chan event.AgentEvent) event.AgentEvent {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for agent event")
		return event.AgentEvent{}
	}
}

func TestManagerTerminalEvents(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
	})

	events, cancel := manager.TerminalBus().Subscribe()
	defer cancel()

	session, err := manager.Create("", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	created := receiveTerminalEvent(t, events)
	if created.Type() != "terminal_created" {
		t.Fatalf("expected terminal_created, got %q", created.Type())
	}
	if created.TerminalID != session.ID {
		t.Fatalf("expected terminal ID %q, got %q", session.ID, created.TerminalID)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	closed := receiveTerminalEvent(t, events)
	if closed.Type() != "terminal_closed" {
		t.Fatalf("expected terminal_closed, got %q", closed.Type())
	}
	if closed.TerminalID != session.ID {
		t.Fatalf("expected terminal ID %q, got %q", session.ID, closed.TerminalID)
	}
}

func receiveTerminalEvent(t *testing.T, ch <-chan event.TerminalEvent) event.TerminalEvent {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for terminal event")
		return event.TerminalEvent{}
	}
}

func TestManagerMultiInstanceKeepsAgentID(t *testing.T) {
	singleton := false
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "/bin/bash",
				Singleton: &singleton,
			},
		},
	})

	first, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	defer func() {
		_ = manager.Delete(first.ID)
	}()

	second, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	defer func() {
		_ = manager.Delete(second.ID)
	}()

	if first.AgentID != "codex" {
		t.Fatalf("expected first agent id codex, got %q", first.AgentID)
	}
	if second.AgentID != "codex" {
		t.Fatalf("expected second agent id codex, got %q", second.AgentID)
	}
	if first.ID == second.ID {
		t.Fatalf("expected unique session ids, got %q", first.ID)
	}
	if !strings.HasPrefix(first.ID, "codex-") || !strings.HasPrefix(second.ID, "codex-") {
		t.Fatalf("expected numbered ids, got %q and %q", first.ID, second.ID)
	}
}

func TestManagerInjectsCodexNotify(t *testing.T) {
	commandFactory := &commandCaptureFactory{}
	config := map[string]interface{}{
		"model":  "o3",
		"notify": []string{"slack"},
	}
	manager := NewManager(ManagerOptions{
		PtyFactory: commandFactory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				CLIType:   "codex",
				CLIConfig: config,
			},
		},
	})

	session, err := manager.Create("codex", "", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	notifyValue, ok := config["notify"].([]string)
	if !ok || len(notifyValue) != 1 || notifyValue[0] != "slack" {
		t.Fatalf("expected original notify config preserved, got %#v", config["notify"])
	}

	notifyArg := ""
	for _, arg := range commandFactory.args {
		if strings.Contains(arg, "notify=") {
			notifyArg = arg
			break
		}
	}
	if notifyArg == "" {
		t.Fatalf("expected notify flag in codex command, got args %#v", commandFactory.args)
	}
	if !strings.Contains(notifyArg, "gestalt-notify") {
		t.Fatalf("expected notify command to include gestalt-notify, got %q", notifyArg)
	}
	if !strings.Contains(notifyArg, "--terminal-id") || !strings.Contains(notifyArg, session.ID) {
		t.Fatalf("expected notify command to include terminal id %q, got %q", session.ID, notifyArg)
	}
	if !strings.Contains(notifyArg, "--agent-id") || !strings.Contains(notifyArg, "codex") {
		t.Fatalf("expected notify command to include agent id codex, got %q", notifyArg)
	}
	if !strings.Contains(notifyArg, "--agent-name") || !strings.Contains(notifyArg, "Codex") {
		t.Fatalf("expected notify command to include agent name Codex, got %q", notifyArg)
	}
}
