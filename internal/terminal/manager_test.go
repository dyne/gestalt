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
	"gestalt/internal/runner/launchspec"
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

type fixedPortResolver struct {
	ports map[string]int
}

func (resolver fixedPortResolver) Get(service string) (int, bool) {
	service = strings.ToLower(strings.TrimSpace(service))
	if service == "" {
		return 0, false
	}
	port, ok := resolver.ports[service]
	return port, ok
}

func TestBuildNotifyArgsUsesFrontendPort(t *testing.T) {
	manager := NewManager(ManagerOptions{
		PortResolver: fixedPortResolver{
			ports: map[string]int{
				"frontend": 60123,
			},
		},
	})
	args := manager.buildNotifyArgs("Coder 1")
	want := []string{"gestalt-notify", "--host", "127.0.0.1", "--port", "60123", "--session-id", "Coder 1"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("expected %v, got %v", want, args)
	}
}

func TestBuildNotifyArgsUsesDefaultPort(t *testing.T) {
	manager := NewManager(ManagerOptions{})
	args := manager.buildNotifyArgs("Coder 1")
	want := []string{"gestalt-notify", "--host", "127.0.0.1", "--port", "57417", "--session-id", "Coder 1"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("expected %v, got %v", want, args)
	}
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

type bridgeTmuxClient struct {
	loads  [][]byte
	pastes []string
}

func (c *bridgeTmuxClient) HasSession(name string) (bool, error) { return true, nil }
func (c *bridgeTmuxClient) HasWindow(sessionName, windowName string) (bool, error) {
	return true, nil
}
func (c *bridgeTmuxClient) SelectWindow(target string) error { return nil }
func (c *bridgeTmuxClient) LoadBuffer(data []byte) error {
	c.loads = append(c.loads, append([]byte(nil), data...))
	return nil
}
func (c *bridgeTmuxClient) PasteBuffer(target string) error {
	c.pastes = append(c.pastes, target)
	return nil
}
func (c *bridgeTmuxClient) ResizePane(target string, cols, rows uint16) error { return nil }

type singletonTmuxClient struct {
	hasSession bool
	windows    map[string]bool
}

func (c *singletonTmuxClient) HasSession(name string) (bool, error) { return c.hasSession, nil }
func (c *singletonTmuxClient) HasWindow(sessionName, windowName string) (bool, error) {
	if c.windows == nil {
		return true, nil
	}
	return c.windows[windowName], nil
}
func (c *singletonTmuxClient) SelectWindow(target string) error                  { return nil }
func (c *singletonTmuxClient) LoadBuffer(data []byte) error                       { return nil }
func (c *singletonTmuxClient) PasteBuffer(target string) error                    { return nil }
func (c *singletonTmuxClient) ResizePane(target string, cols, rows uint16) error { return nil }

type blockingBridgeTmuxClient struct {
	mu               sync.Mutex
	loadCount        int
	loadStarted      chan struct{}
	releaseFirstLoad chan struct{}
	pastes           int
}

func (c *blockingBridgeTmuxClient) HasSession(name string) (bool, error) { return true, nil }
func (c *blockingBridgeTmuxClient) HasWindow(sessionName, windowName string) (bool, error) {
	return true, nil
}
func (c *blockingBridgeTmuxClient) SelectWindow(target string) error { return nil }
func (c *blockingBridgeTmuxClient) LoadBuffer(data []byte) error {
	c.mu.Lock()
	c.loadCount++
	current := c.loadCount
	c.mu.Unlock()
	if current == 1 {
		close(c.loadStarted)
		<-c.releaseFirstLoad
	}
	return nil
}
func (c *blockingBridgeTmuxClient) PasteBuffer(target string) error {
	c.mu.Lock()
	c.pastes++
	c.mu.Unlock()
	return nil
}
func (c *blockingBridgeTmuxClient) ResizePane(target string, cols, rows uint16) error { return nil }

func (c *blockingBridgeTmuxClient) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadCount, c.pastes
}

func TestTmuxManagedSessionWriteUsesTmuxBridge(t *testing.T) {
	tmuxClient := &bridgeTmuxClient{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() TmuxClient { return tmuxClient },
	})

	session, err := manager.CreateWithOptions(CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	payload := []byte("line one\nline two\n")
	if err := session.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if len(tmuxClient.loads) != 1 {
		t.Fatalf("expected one LoadBuffer call, got %d", len(tmuxClient.loads))
	}
	if string(tmuxClient.loads[0]) != string(payload) {
		t.Fatalf("expected payload %q, got %q", string(payload), string(tmuxClient.loads[0]))
	}
	if len(tmuxClient.pastes) != 1 {
		t.Fatalf("expected one PasteBuffer call, got %d", len(tmuxClient.pastes))
	}
	if !strings.HasSuffix(tmuxClient.pastes[0], ":"+session.ID) {
		t.Fatalf("expected paste target to end with %q, got %q", ":"+session.ID, tmuxClient.pastes[0])
	}
}

func TestTmuxManagedSessionWriteSerializesConcurrentBridgeWrites(t *testing.T) {
	tmuxClient := &blockingBridgeTmuxClient{
		loadStarted:      make(chan struct{}),
		releaseFirstLoad: make(chan struct{}),
	}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex", Shell: "codex"},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() TmuxClient { return tmuxClient },
	})

	session, err := manager.CreateWithOptions(CreateOptions{AgentID: "codex"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	firstErr := make(chan error, 1)
	secondErr := make(chan error, 1)
	go func() { firstErr <- session.Write([]byte("first\n")) }()
	select {
	case <-tmuxClient.loadStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for first load-buffer call")
	}

	go func() { secondErr <- session.Write([]byte("second\n")) }()
	time.Sleep(50 * time.Millisecond)
	loadCount, _ := tmuxClient.counts()
	if loadCount != 1 {
		t.Fatalf("expected second write to wait for bridge lock, got %d load-buffer calls", loadCount)
	}

	close(tmuxClient.releaseFirstLoad)
	if err := <-firstErr; err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second write: %v", err)
	}

	loadCount, pasteCount := tmuxClient.counts()
	if loadCount != 2 || pasteCount != 2 {
		t.Fatalf("expected 2 load/paste calls, got loads=%d pastes=%d", loadCount, pasteCount)
	}
}

func TestCreateWithOptionsForcesTmuxRunnerForCodexAgent(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: &fakeFactory{},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "codex",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() TmuxClient { return &bridgeTmuxClient{} },
	})

	session, err := manager.CreateWithOptions(CreateOptions{AgentID: "codex", Runner: "server"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	if session.Runner != string(launchspec.RunnerKindExternal) {
		t.Fatalf("expected runner %q, got %q", launchspec.RunnerKindExternal, session.Runner)
	}
}

func TestCreateWithOptionsPreservesServerRunnerForNonTmuxAgent(t *testing.T) {
	t.Skip("obsolete: all agent sessions are tmux-backed external")
	factory := &captureFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"helper": {
				Name:      "Helper",
				Shell:     "/bin/sh",
				CLIType:   "mock",
				Interface: agent.AgentInterfaceCLI,
			},
		},
	})

	session, err := manager.CreateWithOptions(CreateOptions{AgentID: "helper", Runner: "server"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	if session.Runner != string(launchspec.RunnerKindServer) {
		t.Fatalf("expected runner %q, got %q", launchspec.RunnerKindServer, session.Runner)
	}
	if err := session.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		factory.pty.mu.Lock()
		writes := append([][]byte(nil), factory.pty.writes...)
		factory.pty.mu.Unlock()
		if len(writes) > 0 {
			if string(writes[0]) != "ping\n" {
				t.Fatalf("expected payload %q, got %q", "ping\n", string(writes[0]))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY write")
}

func TestManagerLifecycle(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"coder": {
				Name: "Coder",
			},
			"architect": {
				Name: "Architect",
			},
		},
	})

	first, err := manager.Create("coder", "build", "first")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := manager.Create("architect", "run", "second")
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
	if len(list) != 3 {
		t.Fatalf("expected 3 sessions (2 agents + hub), got %d", len(list))
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

func TestManagerUsesCLIInterfaceForLegacyMCPProfile(t *testing.T) {
	t.Skip("obsolete: legacy mcp/cli branching removed")
	tui := &recordingFactory{pty: &noopPty{}}
	manager := NewManager(ManagerOptions{
		PtyFactory: NewMuxPtyFactory(tui, &recordingFactory{pty: &noopPty{}}, false),
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				CLIType:   "codex",
				Interface: "mcp",
			},
		},
	})

	session, err := manager.Create("codex", "run", "legacy")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	if session.IsMCP() {
		t.Fatalf("expected CLI session")
	}
	if strings.Contains(session.Command, "mcp-server") {
		t.Fatalf("did not expect mcp-server command, got %q", session.Command)
	}
	if !strings.Contains(session.Command, "notify=") {
		t.Fatalf("expected notify in CLI command, got %q", session.Command)
	}
	if session.outputPublisher == nil || session.outputPublisher.policy != OutputBackpressureBlock {
		t.Fatalf("expected output policy block, got %#v", session.outputPublisher)
	}
	if session.outputPublisher == nil || session.outputPublisher.sampleEvery != 0 {
		t.Fatalf("expected sampleEvery=0, got %#v", session.outputPublisher)
	}
}

func TestManagerUsesClock(t *testing.T) {
	factory := &fakeFactory{}
	now := time.Date(2024, 2, 10, 8, 30, 0, 0, time.FixedZone("test", 2*60*60))
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Clock:      fixedClock{now: now},
		Agents: map[string]agent.Agent{
			"codex": {
				Name: "Codex",
			},
		},
	})

	session, err := manager.Create("codex", "build", "clocked")
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

func TestManagerAgentSessionIDSanitizesName(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name: "Bad/Name (Codex)",
			},
		},
	})

	session, err := manager.Create("codex", "build", "first")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID != "BadName (Codex) 1" {
		t.Fatalf("expected id BadName (Codex) 1, got %q", session.ID)
	}
}

func TestManagerAgentSessionIDRejectsEmptyAfterSanitize(t *testing.T) {
	factory := &fakeFactory{}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"bad": {
				Name: " / ",
			},
		},
	})

	if _, err := manager.Create("bad", "build", "first"); err == nil {
		t.Fatalf("expected error for empty sanitized agent name")
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

func TestManagerCodexDeveloperInstructions(t *testing.T) {
	t.Skip("obsolete: codex developer instruction injection removed")
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
		PortResolver: fixedPortResolver{
			ports: map[string]int{
				"otel-grpc": 4319,
			},
		},
		Skills: map[string]*skill.Skill{
			"alpha": {
				Name:        "alpha",
				Description: "Alpha skill",
				Path:        "config/skills/alpha",
			},
		},
		Agents: map[string]agent.Agent{
			"codex": {
				Name:    "Codex",
				Prompts: agent.PromptList{"first"},
				Skills:  []string{"alpha"},
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

	time.Sleep(promptDelay + 100*time.Millisecond)
	factory.pty.mu.Lock()
	writes := len(factory.pty.writes)
	factory.pty.mu.Unlock()
	if writes != 0 {
		t.Fatalf("expected no prompt writes for codex, got %d", writes)
	}

	info := session.Info()
	if len(info.PromptFiles) != 1 || info.PromptFiles[0] != "first.txt" {
		t.Fatalf("unexpected prompt files: %#v", info.PromptFiles)
	}
	if !strings.Contains(session.Command, "developer_instructions=") {
		t.Fatalf("expected developer_instructions in command, got %q", session.Command)
	}
	if !strings.Contains(session.Command, "notify=") {
		t.Fatalf("expected notify in command, got %q", session.Command)
	}
	if !strings.Contains(session.Command, "otel.exporter.otlp-grpc.endpoint=http://127.0.0.1:4319") {
		t.Fatalf("expected otel exporter endpoint in command, got %q", session.Command)
	}
	if !strings.Contains(session.Command, "<available_skills>") {
		t.Fatalf("expected skills XML in command, got %q", session.Command)
	}
	if !strings.Contains(session.Command, "echo hello") {
		t.Fatalf("expected prompt content in command, got %q", session.Command)
	}
}

func TestPromptInjectionTiming_WithMockAgent(t *testing.T) {
	t.Skip("obsolete: server PTY prompt injection path removed for agents")
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
		Agents: map[string]agent.Agent{
			"codex": {
				Name: "Codex",
			},
		},
	})

	session, err := manager.Create("codex", "role", "title")
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
		Agents: map[string]agent.Agent{
			"codex": {Name: "Codex"},
		},
	})

	events, cancel := manager.TerminalBus().Subscribe()
	defer cancel()

	session, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	created := receiveTerminalEventForSession(t, events, session.ID)
	if created.Type() != "terminal_created" {
		t.Fatalf("expected terminal_created, got %q", created.Type())
	}
	if created.TerminalID != session.ID {
		t.Fatalf("expected terminal ID %q, got %q", session.ID, created.TerminalID)
	}

	if err := manager.Delete(session.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	closed := receiveTerminalEventForSession(t, events, session.ID)
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

func receiveTerminalEventForSession(t *testing.T, ch <-chan event.TerminalEvent, sessionID string) event.TerminalEvent {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case evt := <-ch:
			if evt.TerminalID == sessionID {
				return evt
			}
		case <-deadline:
			t.Fatalf("timed out waiting for terminal event for session %q", sessionID)
			return event.TerminalEvent{}
		}
	}
}

func TestManagerSingletonKeepsAgentID(t *testing.T) {
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

	first, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(first.ID)
	}()

	if first.AgentID != "codex" {
		t.Fatalf("expected first agent id codex, got %q", first.AgentID)
	}
	if first.ID != "Codex 1" {
		t.Fatalf("expected canonical id Codex 1, got %q", first.ID)
	}

	_, err = manager.Create("codex", "role", "title")
	if err == nil {
		t.Fatalf("expected duplicate agent error")
	}
	var dup *AgentAlreadyRunningError
	if !errors.As(err, &dup) {
		t.Fatalf("expected AgentAlreadyRunningError, got %v", err)
	}
	if dup.TerminalID != "Codex 1" {
		t.Fatalf("expected conflict terminal id Codex 1, got %q", dup.TerminalID)
	}
}

func TestManagerSingletonReplacesStaleExternalTmuxSession(t *testing.T) {
	factory := &fakeFactory{}
	tmuxClient := &singletonTmuxClient{
		hasSession: true,
		windows:    map[string]bool{"Codex 1": true},
	}
	manager := NewManager(ManagerOptions{
		Shell:      "/bin/sh",
		PtyFactory: factory,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:      "Codex",
				Shell:     "/bin/bash",
				CLIType:   "codex",
				Interface: agent.AgentInterfaceCLI,
			},
		},
		StartExternalTmuxWindow: func(_ *launchspec.LaunchSpec) error { return nil },
		TmuxClientFactory:       func() TmuxClient { return tmuxClient },
	})

	first, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	if first.ID != "Codex 1" {
		t.Fatalf("expected canonical id Codex 1, got %q", first.ID)
	}

	tmuxClient.windows["Codex 1"] = false
	second, err := manager.Create("codex", "role", "title")
	if err != nil {
		t.Fatalf("create replacement session: %v", err)
	}
	defer func() {
		_ = manager.Delete(second.ID)
	}()
	if second.ID != "Codex 1" {
		t.Fatalf("expected canonical id Codex 1, got %q", second.ID)
	}
	if second == first {
		t.Fatalf("expected stale session to be replaced")
	}
}

func TestManagerCloseAllClearsSessions(t *testing.T) {
	manager := NewManager(ManagerOptions{
		PtyFactory:  &fakeFactory{},
		BufferLines: 5,
	})
	startedAt := time.Now()
	sessionOne := newSession("one", newFakePtyWithErr(errors.New("close failed")), nil, nil, "one", "role", startedAt, 5, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	sessionTwo := newSession("two", newFakePty(), nil, nil, "two", "role", startedAt, 5, 0, OutputBackpressureBlock, 0, nil, nil, nil)

	manager.mu.Lock()
	manager.sessions["one"] = sessionOne
	manager.sessions["two"] = sessionTwo
	manager.agentSessions["agent"] = "one"
	manager.mu.Unlock()

	if err := manager.CloseAll(); err == nil {
		t.Fatalf("expected close error")
	}
	if sessionOne.State() != sessionStateClosed {
		t.Fatalf("expected session one closed")
	}
	if sessionTwo.State() != sessionStateClosed {
		t.Fatalf("expected session two closed")
	}
	if len(manager.sessions) != 0 {
		t.Fatalf("expected sessions cleared")
	}
	if len(manager.agentSessions) != 0 {
		t.Fatalf("expected agent sessions cleared")
	}
}
