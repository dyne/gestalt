package agent_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gestalt/internal/agent"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
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
	_ = p.writer.Close()
	return nil
}

func (p *fakePty) Resize(cols, rows uint16) error {
	return nil
}

type commandCaptureFactory struct {
	command string
	args    []string
}

func (f *commandCaptureFactory) Start(command string, args ...string) (terminal.Pty, *exec.Cmd, error) {
	f.command = command
	f.args = append([]string(nil), args...)
	return newFakePty(), nil, nil
}

func TestIntegrationLoadAgentsAndValidate(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	agentsDir := filepath.Join(wd, "testdata")

	buffer := logging.NewLogBuffer(20)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := agent.Loader{Logger: logger}
	agents, err := loader.Load(nil, agentsDir, "", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 valid agents, got %d", len(agents))
	}
	codex, ok := agents["valid-codex"]
	if !ok {
		t.Fatalf("missing valid-codex")
	}
	if strings.TrimSpace(codex.ConfigHash) == "" {
		t.Fatalf("expected config hash")
	}
	if codex.Shell != "codex -c approval_policy=never -c model=o3" {
		t.Fatalf("unexpected shell: %q", codex.Shell)
	}
	if _, ok := agents["valid-copilot"]; !ok {
		t.Fatalf("missing valid-copilot")
	}

	entries := buffer.List()
	if !hasLogError(entries, "model") {
		t.Fatalf("expected model type error")
	}
	if !hasLogError(entries, "unknown_field") {
		t.Fatalf("expected unknown field error")
	}
	if !hasLogError(entries, "only TOML agent configs are supported") {
		t.Fatalf("expected JSON rejection warning")
	}
}

func TestIntegrationCreateSessionFromTOML(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	agentsDir := filepath.Join(wd, "testdata")

	loader := agent.Loader{}
	agents, err := loader.Load(nil, agentsDir, "", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	codex := agents["valid-codex"]

	factory := &commandCaptureFactory{}
	manager := terminal.NewManager(terminal.ManagerOptions{
		PtyFactory: factory,
		Agents:     agents,
		AgentsDir:  agentsDir,
	})
	session, err := manager.Create("valid-codex", "run", "integration")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	if factory.command != "codex" {
		t.Fatalf("expected command codex, got %q", factory.command)
	}
	wantArgs := []string{"mcp-server", "-c", "approval_policy=never", "-c", "model=o3"}
	if len(factory.args) < len(wantArgs) {
		t.Fatalf("expected args to include %v, got %v", wantArgs, factory.args)
	}
	for i, arg := range wantArgs {
		if factory.args[i] != arg {
			t.Fatalf("expected args %v, got %v", wantArgs, factory.args)
		}
	}
	for _, arg := range factory.args {
		if strings.Contains(arg, "notify=") {
			t.Fatalf("did not expect notify config in args, got %v", factory.args)
		}
	}
	if session.ConfigHash != codex.ConfigHash {
		t.Fatalf("expected config hash %q, got %q", codex.ConfigHash, session.ConfigHash)
	}
	if !strings.Contains(session.Command, "mcp-server") {
		t.Fatalf("expected mcp-server in session command, got %q", session.Command)
	}
}

func hasLogError(entries []logging.LogEntry, needle string) bool {
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && strings.Contains(entry.Context["error"], needle) {
			return true
		}
	}
	return false
}
