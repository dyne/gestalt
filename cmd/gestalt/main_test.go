package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gestalt/internal/app"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
)

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("GESTALT_PORT", "9090")
	t.Setenv("GESTALT_BACKEND_PORT", "9091")
	t.Setenv("GESTALT_SHELL", "/bin/zsh")
	t.Setenv("GESTALT_TOKEN", "secret")
	t.Setenv("GESTALT_SESSION_RETENTION_DAYS", "9")
	t.Setenv("GESTALT_SESSION_PERSIST", "true")
	t.Setenv("GESTALT_SESSION_DIR", "/tmp/gestalt-logs")
	t.Setenv("GESTALT_SESSION_BUFFER_LINES", "2048")
	t.Setenv("GESTALT_INPUT_HISTORY_PERSIST", "true")
	t.Setenv("GESTALT_INPUT_HISTORY_DIR", "/tmp/gestalt-input")

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FrontendPort != 9090 {
		t.Fatalf("expected frontend port 9090, got %d", cfg.FrontendPort)
	}
	if cfg.BackendPort != 9091 {
		t.Fatalf("expected backend port 9091, got %d", cfg.BackendPort)
	}
	if cfg.Shell != "/bin/zsh" {
		t.Fatalf("expected shell /bin/zsh, got %q", cfg.Shell)
	}
	if cfg.AuthToken != "secret" {
		t.Fatalf("expected token secret, got %q", cfg.AuthToken)
	}
	if cfg.SessionRetentionDays != 9 {
		t.Fatalf("expected retention days 9, got %d", cfg.SessionRetentionDays)
	}
	if !cfg.SessionPersist {
		t.Fatalf("expected session persistence true")
	}
	if cfg.SessionLogDir != "/tmp/gestalt-logs" {
		t.Fatalf("expected session log dir /tmp/gestalt-logs, got %q", cfg.SessionLogDir)
	}
	if cfg.SessionBufferLines != 2048 {
		t.Fatalf("expected session buffer lines 2048, got %d", cfg.SessionBufferLines)
	}
	if !cfg.InputHistoryPersist {
		t.Fatalf("expected input history persistence true")
	}
	if cfg.InputHistoryDir != "/tmp/gestalt-input" {
		t.Fatalf("expected input history dir /tmp/gestalt-input, got %q", cfg.InputHistoryDir)
	}
}

func TestLoadConfigDefaultsOnInvalidPort(t *testing.T) {
	t.Setenv("GESTALT_PORT", "not-a-number")
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.FrontendPort != 57417 {
		t.Fatalf("expected default frontend port 57417, got %d", cfg.FrontendPort)
	}
	if cfg.BackendPort != 0 {
		t.Fatalf("expected backend port 0, got %d", cfg.BackendPort)
	}
	if cfg.SessionLogDir != filepath.Join(".gestalt", "sessions") {
		t.Fatalf("expected default session log dir, got %q", cfg.SessionLogDir)
	}
	if cfg.SessionBufferLines != 1000 {
		t.Fatalf("expected default session buffer lines 1000, got %d", cfg.SessionBufferLines)
	}
	if cfg.InputHistoryDir != filepath.Join(".gestalt", "input-history") {
		t.Fatalf("expected default input history dir, got %q", cfg.InputHistoryDir)
	}
}

func TestLoadConfigDisablesSessionPersistence(t *testing.T) {
	t.Setenv("GESTALT_SESSION_PERSIST", "false")
	t.Setenv("GESTALT_SESSION_DIR", "/tmp/gestalt-logs")

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SessionPersist {
		t.Fatalf("expected session persistence disabled")
	}
	if cfg.SessionLogDir != "" {
		t.Fatalf("expected empty session log dir when disabled, got %q", cfg.SessionLogDir)
	}
}

func TestLoadConfigDisablesInputHistory(t *testing.T) {
	t.Setenv("GESTALT_INPUT_HISTORY_PERSIST", "false")
	t.Setenv("GESTALT_INPUT_HISTORY_DIR", "/tmp/gestalt-input")

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.InputHistoryPersist {
		t.Fatalf("expected input history persistence disabled")
	}
	if cfg.InputHistoryDir != "" {
		t.Fatalf("expected empty input history dir when disabled, got %q", cfg.InputHistoryDir)
	}
}

func TestFindStaticDir(t *testing.T) {
	root := t.TempDir()
	overrideDist := filepath.Join(root, "gestalt", "dist")
	if err := os.MkdirAll(overrideDist, 0755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := findStaticDir(); got != filepath.Join("gestalt", "dist") {
		t.Fatalf("expected %q, got %q", filepath.Join("gestalt", "dist"), got)
	}
}

func TestLoadAgentsIntegration(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".gestalt", "config", "agents")
	promptsDir := filepath.Join(root, ".gestalt", "config", "prompts")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "hello.tmpl"), []byte("echo hi\n"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	agentTOML := "name = \"Codex\"\nshell = \"/bin/bash\"\nprompt = \"hello\"\ncli_type = \"codex\"\nllm_model = \"default\"\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "codex.toml"), []byte(agentTOML), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	logger := logging.NewLoggerWithOutput(logging.NewLogBuffer(10), logging.LevelInfo, io.Discard)
	configFS := buildConfigFS(filepath.Join(root, ".gestalt"))
	agents, err := app.LoadAgents(logger, configFS, "config", nil)
	if err != nil {
		t.Fatalf("load agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents["codex"].Name != "Codex" {
		t.Fatalf("expected Codex, got %q", agents["codex"].Name)
	}
}

func TestLoadAgentsReportsInvalidTOML(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".gestalt", "config", "agents")
	promptsDir := filepath.Join(root, ".gestalt", "config", "prompts")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.toml"), []byte("name = \"Bad\"\\nmodel = {"), 0644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
	configFS := buildConfigFS(filepath.Join(root, ".gestalt"))
	agents, err := app.LoadAgents(logger, configFS, "config", nil)
	if err != nil {
		t.Fatalf("load agents: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected no agents, got %d", len(agents))
	}
	if !hasLogMessage(buffer.List(), "agent load failed") {
		t.Fatalf("expected warning for invalid agent toml")
	}
}

func TestPreparePlanFileCreatesPlansDir(t *testing.T) {
	withTempDir(t, func(root string) {
		content := "* TODO [#A] Example\n"
		legacyPath := filepath.Join(root, "PLAN.org")
		if err := os.WriteFile(legacyPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write legacy plan: %v", err)
		}

		buffer := logging.NewLogBuffer(10)
		logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
		plansDir := preparePlanFile(logger)
		if plansDir != plan.DefaultPlansDir() {
			t.Fatalf("expected plans dir %q, got %q", plan.DefaultPlansDir(), plansDir)
		}

		fullPath := filepath.Join(root, plansDir)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("expected plans dir to exist: %v", err)
		}
		legacyData, err := os.ReadFile(legacyPath)
		if err != nil {
			t.Fatalf("read legacy plan: %v", err)
		}
		if string(legacyData) != content {
			t.Fatalf("expected legacy plan to remain unchanged")
		}
		if hasLogMessage(buffer.List(), "Migrated PLAN.org") {
			t.Fatalf("did not expect migration log entry")
		}
	})
}

func TestLogVersionInfo(t *testing.T) {
	buffer := logging.NewLogBuffer(5)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
	logVersionInfo(logger)

	entries := buffer.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Message, "Gestalt version ") {
		t.Fatalf("unexpected log message %q", entries[0].Message)
	}
}

func withTempDir(t *testing.T, fn func(root string)) {
	t.Helper()
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	fn(root)
}

func hasLogMessage(entries []logging.LogEntry, message string) bool {
	for _, entry := range entries {
		if entry.Message == message {
			return true
		}
	}
	return false
}
