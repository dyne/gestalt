package terminal

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gestalt/internal/agent"
)

func TestSessionWriteAndOutput(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	out, cancel := session.Subscribe()
	defer cancel()

	pty.Emit("hello\n")
	if !receiveChunk(t, out, []byte("hello\n")) {
		t.Fatalf("expected to receive output chunk")
	}

	if err := session.Write([]byte("ls\n")); err != nil {
		t.Fatalf("write session: %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		pty.mu.Lock()
		writes := append([][]byte(nil), pty.writes...)
		pty.mu.Unlock()
		if len(writes) > 0 {
			if string(writes[0]) != "ls\n" {
				t.Fatalf("expected write ls\\n, got %q", string(writes[0]))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY write")
}

func TestSessionFiltersANSIOutput(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	out, cancel := session.Subscribe()
	defer cancel()

	pty.Emit("hello\x1b[31mred\x1b[0m\n")
	if !receiveChunk(t, out, []byte("hellored\n")) {
		t.Fatalf("expected filtered output chunk")
	}
}

func TestSessionCloseTransitionsState(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if session.State() != sessionStateClosed {
		t.Fatalf("expected state closed, got %v", session.State())
	}
}

func TestSessionWriteAfterClose(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected no panic, got %v", r)
		}
	}()

	if err := session.Write([]byte("ls\n")); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed, got %v", err)
	}
}

func TestSessionWriteRoutesToRunner(t *testing.T) {
	runner := &captureRunner{}
	session := newSession("1", nil, runner, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)

	if err := session.Write([]byte("ls\n")); err != nil {
		t.Fatalf("write session: %v", err)
	}
	if err := session.Resize(120, 40); err != nil {
		t.Fatalf("resize session: %v", err)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.writes) != 1 || string(runner.writes[0]) != "ls\n" {
		t.Fatalf("expected runner write, got %#v", runner.writes)
	}
	if runner.resizeCols != 120 || runner.resizeRows != 40 {
		t.Fatalf("expected resize 120x40, got %dx%d", runner.resizeCols, runner.resizeRows)
	}
}

func TestSessionPublishOutputWithoutPty(t *testing.T) {
	session := newSession("1", nil, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	out, cancel := session.Subscribe()
	defer cancel()

	session.PublishOutputChunk([]byte("hello\n"))
	if !receiveChunk(t, out, []byte("hello\n")) {
		t.Fatalf("expected to receive output chunk")
	}
}

type captureRunner struct {
	mu         sync.Mutex
	writes     [][]byte
	resizeCols uint16
	resizeRows uint16
}

func (r *captureRunner) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, append([]byte(nil), data...))
	return nil
}

func (r *captureRunner) Resize(cols, rows uint16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resizeCols = cols
	r.resizeRows = rows
	return nil
}

func (r *captureRunner) Close() error {
	return nil
}

func TestSessionAutoRespondsToCursorPosition(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	pty.Emit("\x1b[6n")

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		pty.mu.Lock()
		writes := append([][]byte(nil), pty.writes...)
		pty.mu.Unlock()
		for _, write := range writes {
			if string(write) == "\x1b[1;1R" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected cursor position response")
}

func TestSessionFallbacksCursorPositionWithSubscriber(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	_, cancel := session.Subscribe()
	defer cancel()

	pty.Emit("\x1b[6n")

	deadline := time.Now().Add(dsrFallbackDelay + 150*time.Millisecond)
	for time.Now().Before(deadline) {
		pty.mu.Lock()
		writes := append([][]byte(nil), pty.writes...)
		pty.mu.Unlock()
		for _, write := range writes {
			if string(write) == "\x1b[1;1R" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected fallback cursor position response")
}

func TestSessionRecordsInputHistory(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	session.RecordInput(" ls ")
	session.RecordInput("   ")

	entries := session.GetInputHistory()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", entries)
	}
	if entries[0].Command != "ls" {
		t.Fatalf("expected command ls, got %q", entries[0].Command)
	}
	if entries[0].Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

func TestSessionInfoIncludesMetadata(t *testing.T) {
	profile := &agent.Agent{
		Name:       "Codex",
		CLIType:    "codex",
		LLMModel:   "o3",
		Skills:     []string{"skill-a", "skill-b"},
		GUIModules: []string{"plan-progress"},
	}
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, profile, nil, nil)
	session.Command = "codex -c model=o3"
	session.PromptFiles = []string{"prompt-a", "prompt-b"}
	defer func() {
		_ = session.Close()
	}()

	info := session.Info()
	if info.ID != "1" {
		t.Fatalf("expected id 1, got %q", info.ID)
	}
	if info.Title != "title" || info.Role != "role" {
		t.Fatalf("unexpected meta: %#v", info)
	}
	if info.LLMType != "codex" || info.LLMModel != "o3" {
		t.Fatalf("unexpected llm info: %#v", info)
	}
	if info.Interface != agent.AgentInterfaceCLI {
		t.Fatalf("expected interface %q, got %q", agent.AgentInterfaceCLI, info.Interface)
	}
	if info.Command != session.Command {
		t.Fatalf("expected command %q, got %q", session.Command, info.Command)
	}
	if len(info.Skills) != 2 || info.Skills[0] != "skill-a" || info.Skills[1] != "skill-b" {
		t.Fatalf("unexpected skills: %v", info.Skills)
	}
	if len(info.PromptFiles) != 2 || info.PromptFiles[0] != "prompt-a" || info.PromptFiles[1] != "prompt-b" {
		t.Fatalf("unexpected prompt files: %v", info.PromptFiles)
	}
	if len(info.GUIModules) != 1 || info.GUIModules[0] != "plan-progress" {
		t.Fatalf("unexpected gui modules: %v", info.GUIModules)
	}
}

func TestSessionWorkflowIdentifiersEmpty(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	workflowID, runID, ok := session.WorkflowIdentifiers()
	if ok {
		t.Fatalf("expected no workflow identifiers, got %q %q", workflowID, runID)
	}
	if workflowID != "" || runID != "" {
		t.Fatalf("expected empty identifiers, got %q %q", workflowID, runID)
	}
}

func TestSessionResizePropagatesToOutputFilter(t *testing.T) {
	t.Setenv(envTerminalOutputFilters, "scrollback-vt")

	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, 0, OutputBackpressureBlock, 0, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	chain, ok := session.outputFilter.(*FilterChain)
	if !ok || len(chain.filters) != 1 {
		t.Fatalf("expected scrollback filter chain, got %#v", session.outputFilter)
	}
	scrollback, ok := chain.filters[0].(*scrollbackVTFilter)
	if !ok {
		t.Fatalf("expected scrollback filter, got %#v", chain.filters[0])
	}

	if err := session.Resize(120, 40); err != nil {
		t.Fatalf("resize session: %v", err)
	}
	if scrollback.cols != 120 || scrollback.rows != 40 {
		t.Fatalf("expected resize to update filter, got %d x %d", scrollback.cols, scrollback.rows)
	}
}
