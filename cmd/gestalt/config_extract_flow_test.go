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
	"gestalt/internal/config"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
)

func TestPrepareConfigWarmStartSkipsExtraction(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelDebug)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	agentPath := filepath.Join(root, cfg.ConfigDir, "agents", "coder.toml")
	firstInfo, err := os.Stat(agentPath)
	if err != nil {
		t.Fatalf("stat extracted agent: %v", err)
	}

	logger = newTestLogger(logging.LevelDebug)
	if _, err := prepareConfig(cfg, logger); err != nil {
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

func TestPrepareConfigConflictKeepsWithDist(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	agentPath := filepath.Join(root, cfg.ConfigDir, "agents", "coder.toml")
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
	baseline, err := config.LoadBaselineManifest(cfg.ConfigDir)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	baseline["agents/coder.toml"] = "0000000000000000"
	if err := config.WriteBaselineManifest(cfg.ConfigDir, baseline); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	originalStdin := os.Stdin
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	os.Stdin = pipeReader
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
	})

	logger = newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	distPath := agentPath + ".dist"
	dist, err := os.ReadFile(distPath)
	if err != nil {
		t.Fatalf("read dist: %v", err)
	}
	expected, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/agents/coder.toml")
	if err != nil {
		t.Fatalf("read embedded agent: %v", err)
	}
	if string(dist) != string(expected) {
		t.Fatalf("expected dist to match embedded contents")
	}
	current, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read extracted agent: %v", err)
	}
	if string(current) != "custom" {
		t.Fatalf("expected custom contents to remain")
	}
	if _, err := os.Stat(agentPath + ".bck"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file presence: %v", err)
	}
}

func TestPrepareConfigPartialExtraction(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	promptPath := filepath.Join(root, cfg.ConfigDir, "prompts", "architect.tmpl")
	if err := os.Remove(promptPath); err != nil {
		t.Fatalf("remove prompt: %v", err)
	}

	logger = newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	if _, err := os.Stat(promptPath); err != nil {
		t.Fatalf("expected prompt to be re-extracted: %v", err)
	}
	expected, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/prompts/architect.tmpl")
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

func TestPrepareConfigReextractsMissingDefaultFlowFile(t *testing.T) {
	root := withTempWorkdir(t)

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	flowPath := filepath.Join(root, cfg.ConfigDir, "flows", "default-file-changed.flow.yaml")
	if err := os.Remove(flowPath); err != nil {
		t.Fatalf("remove flow file: %v", err)
	}

	logger = newTestLogger(logging.LevelInfo)
	if _, err := prepareConfig(cfg, logger); err != nil {
		t.Fatalf("prepare config: %v", err)
	}

	extracted, err := os.ReadFile(flowPath)
	if err != nil {
		t.Fatalf("read extracted flow file: %v", err)
	}
	expected, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/flows/default-file-changed.flow.yaml")
	if err != nil {
		t.Fatalf("read embedded flow file: %v", err)
	}
	if string(extracted) != string(expected) {
		t.Fatalf("expected extracted flow file to match embedded contents")
	}
}

func TestPreparePlanFileMigration(t *testing.T) {
	root := withTempWorkdir(t)

	payload := []byte("* Test Plan\n")
	if err := os.WriteFile(filepath.Join(root, "PLAN.org"), payload, 0o644); err != nil {
		t.Fatalf("write PLAN.org: %v", err)
	}

	logger := newTestLogger(logging.LevelInfo)
	plansDir := preparePlanFile(logger)
	if plansDir != plan.DefaultPlansDir() {
		t.Fatalf("expected plans dir %q, got %q", plan.DefaultPlansDir(), plansDir)
	}

	if _, err := os.Stat(filepath.Join(root, plansDir)); err != nil {
		t.Fatalf("expected plans dir to exist: %v", err)
	}
	legacy, err := os.ReadFile(filepath.Join(root, "PLAN.org"))
	if err != nil {
		t.Fatalf("read legacy plan: %v", err)
	}
	if string(legacy) != string(payload) {
		t.Fatalf("expected legacy plan contents to remain")
	}
	if logContains(logger.Buffer(), "Migrated PLAN.org") {
		t.Fatalf("did not expect migration log entry")
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
