package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"gestalt"
	"gestalt/internal/logging"
)

func TestLoaderMissingDir(t *testing.T) {
	loader := Loader{}
	agents, err := loader.Load(nil, filepath.Join(t.TempDir(), "missing"), "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected empty map, got %d", len(agents))
	}
}

func TestLoaderValidAgents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
cli_type = "codex"
llm_model = "default"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`ignore`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loader := Loader{}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	agent, ok := agents["codex"]
	if !ok {
		t.Fatalf("missing codex agent")
	}
	if agent.Name != "Codex" {
		t.Fatalf("name mismatch: %q", agent.Name)
	}
}

func TestLoaderInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.toml"), []byte(`name = "Bad"\nmodel = {`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
	if !hasAgentWarning(buffer.List(), "agent load failed") {
		t.Fatalf("expected warning for invalid agent toml")
	}
}

func TestLoaderDuplicateAgentName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.toml"), []byte(`
name = "Coder"
shell = "/bin/bash"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.toml"), []byte(`
name = "coder"
shell = "/bin/zsh"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if _, ok := agents["alpha"]; !ok {
		t.Fatalf("expected alpha agent to be kept")
	}
	if !hasAgentWarning(buffer.List(), "agent duplicate name ignored") {
		t.Fatalf("expected warning for duplicate agent name")
	}
}

func TestLoaderInvalidAgent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.toml"), []byte(`
name = "Bad"
shell = " "
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
	if !hasAgentWarning(buffer.List(), "agent load failed") {
		t.Fatalf("expected warning for invalid agent")
	}
}

func TestLoaderMissingPromptLogsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
prompt = ["missing"]
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	promptsDir := t.TempDir()

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	if _, err := loader.Load(nil, dir, promptsDir, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "agent prompt file missing" {
			found = true
			if entry.Context["prompt"] != "missing" {
				t.Fatalf("prompt context mismatch: %q", entry.Context["prompt"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for missing prompt")
	}
}

func TestLoaderPromptResolutionSupportsMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
prompt = ["notes", "explicit.md"]
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	promptsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptsDir, "notes.md"), []byte("notes"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "explicit.md"), []byte("explicit"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	if _, err := loader.Load(nil, dir, promptsDir, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warning := findAgentWarning(buffer.List(), "agent prompt file missing"); warning != nil {
		t.Fatalf("unexpected prompt warning: %v", warning.Context)
	}
}

func TestLoaderMissingSkillLogsWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
skills = ["git-workflows", "missing-skill"]
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	skillIndex := map[string]struct{}{
		"git-workflows": {},
	}
	agents, err := loader.Load(nil, dir, "", skillIndex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agent, ok := agents["codex"]
	if !ok {
		t.Fatalf("missing codex agent")
	}
	if len(agent.Skills) != 1 || agent.Skills[0] != "git-workflows" {
		t.Fatalf("skills mismatch: %v", agent.Skills)
	}

	entries := buffer.List()
	if len(entries) == 0 {
		t.Fatalf("expected warning log entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == "agent skill missing" {
			found = true
			if entry.Context["skill"] != "missing-skill" {
				t.Fatalf("skill context mismatch: %q", entry.Context["skill"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected warning log for missing skill")
	}
}

func TestLoaderRejectsJSONFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy.json"), []byte(`{"name":"Legacy","shell":"/bin/bash"}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if !hasAgentWarning(buffer.List(), "agent load failed") {
		t.Fatalf("expected warning for json agent")
	}
}

func TestLoaderSchemaViolationMessage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
cli_type = "codex"
model = 123
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
	entry := findAgentWarning(buffer.List(), "agent load failed")
	if entry == nil {
		t.Fatalf("expected warning for schema violation")
	}
	if !strings.Contains(entry.Context["error"], "model") {
		t.Fatalf("expected model in error, got %q", entry.Context["error"])
	}
}

func TestLoaderWarnsOnDeprecatedLLMModel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
llm_model = "default"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(nil, dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	entry := findAgentWarning(buffer.List(), "agent config warning")
	if entry == nil {
		t.Fatalf("expected warning for deprecated llm_model")
	}
	if !strings.Contains(entry.Context["warning"], "llm_model") {
		t.Fatalf("expected warning to mention llm_model, got %q", entry.Context["warning"])
	}
}

func TestLoaderWithFS(t *testing.T) {
	fsys := fstest.MapFS{
		"config/agents/codex.toml": &fstest.MapFile{
			Data: []byte(`
name = "Codex"
shell = "/bin/bash"
prompt = ["init"]
`),
		},
		"config/prompts/init.tmpl": &fstest.MapFile{
			Data: []byte("echo ok"),
		},
	}

	loader := Loader{}
	agents, err := loader.Load(fsys, "config/agents", "config/prompts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if _, ok := agents["codex"]; !ok {
		t.Fatalf("missing codex agent")
	}
}

func TestLoaderEmbeddedAgents(t *testing.T) {
	buffer := logging.NewLogBuffer(20)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, nil)
	loader := Loader{Logger: logger}
	agents, err := loader.Load(gestalt.EmbeddedConfigFS, "config/agents", "config/prompts", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) == 0 {
		t.Fatalf("expected embedded agents to load")
	}
	for _, entry := range buffer.List() {
		if entry.Level == logging.LevelWarning || entry.Level == logging.LevelError {
			t.Fatalf("unexpected loader warning: %s", entry.Message)
		}
	}
}

func TestLoadAgentByID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), []byte(`
name = "Codex"
shell = "/bin/bash"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	agent, err := LoadAgentByID("codex", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Name != "Codex" {
		t.Fatalf("name mismatch: %q", agent.Name)
	}
	if strings.TrimSpace(agent.ConfigHash) == "" {
		t.Fatalf("expected config hash to be set")
	}
}

func TestLoadAgentByIDMissing(t *testing.T) {
	_, err := LoadAgentByID("missing", t.TempDir())
	if err == nil {
		t.Fatalf("expected error")
	}
}

func hasAgentWarning(entries []logging.LogEntry, message string) bool {
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == message {
			return true
		}
	}
	return false
}

func findAgentWarning(entries []logging.LogEntry, message string) *logging.LogEntry {
	for _, entry := range entries {
		if entry.Level == logging.LevelWarning && entry.Message == message {
			return &entry
		}
	}
	return nil
}
