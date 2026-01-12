package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gestalt"
	"gestalt/internal/logging"
	"gestalt/internal/server"
)

func TestPrepareConfigWarmStartSkipsExtraction(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := server.LoadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelDebug)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	agentPath := filepath.Join(root, cfg.ConfigDir, "agents", "codex.json")
	firstInfo, err := os.Stat(agentPath)
	if err != nil {
		t.Fatalf("stat extracted agent: %v", err)
	}

	logger = newTestLogger(logging.LevelDebug)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	secondInfo, err := os.Stat(agentPath)
	if err != nil {
		t.Fatalf("stat extracted agent: %v", err)
	}
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Fatalf("expected mod time to remain %v, got %v", firstInfo.ModTime(), secondInfo.ModTime())
	}
	if !logContains(logger.Buffer(), "config file unchanged since last extraction, skipping") &&
		!logContains(logger.Buffer(), "config file up-to-date, skipping") {
		t.Fatalf("expected skip log entry")
	}
	if !logContains(logger.Buffer(), "config extraction metrics") {
		t.Fatalf("expected metrics log entry")
	}
}

func TestPrepareConfigConflictBacksUp(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := server.LoadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	agentPath := filepath.Join(root, cfg.ConfigDir, "agents", "codex.json")
	if err := os.Chmod(agentPath, 0o644); err != nil {
		t.Fatalf("chmod agent: %v", err)
	}
	if err := os.WriteFile(agentPath, []byte("custom"), 0o644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(agentPath, future, future); err != nil {
		t.Fatalf("set mod time: %v", err)
	}

	logger = newTestLogger(logging.LevelInfo)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	backupPath := agentPath + ".bck"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "custom" {
		t.Fatalf("expected backup to contain custom data")
	}

	expected, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/agents/codex.json")
	if err != nil {
		t.Fatalf("read embedded agent: %v", err)
	}
	current, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read extracted agent: %v", err)
	}
	if string(current) != string(expected) {
		t.Fatalf("expected extracted agent to match embedded contents")
	}
	if !logContains(logger.Buffer(), "config file backed up") {
		t.Fatalf("expected backup log entry")
	}
}

func TestPrepareConfigPartialExtraction(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := server.LoadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	promptPath := filepath.Join(root, cfg.ConfigDir, "prompts", "architect.txt")
	if err := os.Remove(promptPath); err != nil {
		t.Fatalf("remove prompt: %v", err)
	}

	logger = newTestLogger(logging.LevelInfo)
	if _, err := server.PrepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	if _, err := os.Stat(promptPath); err != nil {
		t.Fatalf("expected prompt to be re-extracted: %v", err)
	}
	expected, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/prompts/architect.txt")
	if err != nil {
		t.Fatalf("read embedded prompt: %v", err)
	}
	current, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read extracted prompt: %v", err)
	}
	if string(current) != string(expected) {
		t.Fatalf("expected extracted prompt to match embedded contents")
	}
	if !logContains(logger.Buffer(), "config file extracted") {
		t.Fatalf("expected extraction log entry")
	}
}

func TestPreparePlanFileMigration(t *testing.T) {
	root := withTempWorkdir(t)

	payload := []byte("* Test Plan\n")
	if err := os.WriteFile(filepath.Join(root, "PLAN.org"), payload, 0o644); err != nil {
		t.Fatalf("write PLAN.org: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	planPath := server.PreparePlanFile(logger)
	if planPath != filepath.Join(".gestalt", "PLAN.org") {
		t.Fatalf("expected plan path .gestalt/PLAN.org, got %q", planPath)
	}

	contents, err := os.ReadFile(filepath.Join(root, planPath))
	if err != nil {
		t.Fatalf("read migrated plan: %v", err)
	}
	if string(contents) != string(payload) {
		t.Fatalf("expected plan contents to match")
	}
	if !logContains(logger.Buffer(), "Migrated PLAN.org to .gestalt/PLAN.org") {
		t.Fatalf("expected migration log entry")
	}
}

func withTempWorkdir(t *testing.T) string {
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
	return root
}

func newTestLogger(level logging.Level) *logging.Logger {
	buffer := logging.NewLogBuffer(5000)
	return logging.NewLoggerWithOutput(buffer, level, io.Discard)
}

func logContains(buffer *logging.LogBuffer, message string) bool {
	if buffer == nil {
		return false
	}
	for _, entry := range buffer.List() {
		if strings.Contains(entry.Message, message) {
			return true
		}
	}
	return false
}
