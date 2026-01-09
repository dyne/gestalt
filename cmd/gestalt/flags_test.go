package main

import (
	"errors"
	"flag"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Shell == "" {
		t.Fatalf("expected default shell to be set")
	}
	if !cfg.SessionPersist {
		t.Fatalf("expected session persistence enabled")
	}
	if cfg.SessionLogDir != filepath.Join(".gestalt", "sessions") {
		t.Fatalf("expected default session dir, got %q", cfg.SessionLogDir)
	}
	if cfg.SessionBufferLines != 1000 {
		t.Fatalf("expected default buffer lines 1000, got %d", cfg.SessionBufferLines)
	}
	if !cfg.InputHistoryPersist {
		t.Fatalf("expected input history persistence enabled")
	}
	if cfg.InputHistoryDir != filepath.Join(".gestalt", "input-history") {
		t.Fatalf("expected default input history dir, got %q", cfg.InputHistoryDir)
	}
	if cfg.MaxWatches != 100 {
		t.Fatalf("expected max watches 100, got %d", cfg.MaxWatches)
	}
	if cfg.TemporalDevServer {
		t.Fatalf("expected temporal dev server disabled by default")
	}
	if cfg.Verbose {
		t.Fatalf("expected verbose false by default")
	}
	if cfg.Quiet {
		t.Fatalf("expected quiet false by default")
	}
}

func TestLoadConfigEnvOverridesDefaults(t *testing.T) {
	t.Setenv("GESTALT_PORT", "9090")
	t.Setenv("GESTALT_SHELL", "/bin/zsh")
	t.Setenv("GESTALT_TOKEN", "secret")
	t.Setenv("GESTALT_SESSION_RETENTION_DAYS", "9")
	t.Setenv("GESTALT_SESSION_PERSIST", "false")
	t.Setenv("GESTALT_SESSION_DIR", "/tmp/gestalt-logs")
	t.Setenv("GESTALT_SESSION_BUFFER_LINES", "2048")
	t.Setenv("GESTALT_INPUT_HISTORY_PERSIST", "false")
	t.Setenv("GESTALT_INPUT_HISTORY_DIR", "/tmp/gestalt-input")
	t.Setenv("GESTALT_MAX_WATCHES", "55")
	t.Setenv("GESTALT_TEMPORAL_DEV_SERVER", "true")

	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
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
	if cfg.SessionPersist {
		t.Fatalf("expected session persistence disabled")
	}
	if cfg.SessionLogDir != "" {
		t.Fatalf("expected session log dir empty when disabled, got %q", cfg.SessionLogDir)
	}
	if cfg.SessionBufferLines != 2048 {
		t.Fatalf("expected session buffer lines 2048, got %d", cfg.SessionBufferLines)
	}
	if cfg.InputHistoryPersist {
		t.Fatalf("expected input history disabled")
	}
	if cfg.InputHistoryDir != "" {
		t.Fatalf("expected input history dir empty when disabled, got %q", cfg.InputHistoryDir)
	}
	if cfg.MaxWatches != 55 {
		t.Fatalf("expected max watches 55, got %d", cfg.MaxWatches)
	}
	if !cfg.TemporalDevServer {
		t.Fatalf("expected temporal dev server enabled")
	}
}

func TestLoadConfigFlagOverridesEnv(t *testing.T) {
	t.Setenv("GESTALT_PORT", "9090")
	t.Setenv("GESTALT_SHELL", "/bin/zsh")
	t.Setenv("GESTALT_SESSION_PERSIST", "false")
	t.Setenv("GESTALT_SESSION_DIR", "/tmp/gestalt-logs")
	t.Setenv("GESTALT_SESSION_BUFFER_LINES", "400")
	t.Setenv("GESTALT_MAX_WATCHES", "50")
	t.Setenv("GESTALT_TEMPORAL_DEV_SERVER", "false")

	cfg, err := loadConfig([]string{
		"--port", "7070",
		"--shell", "/bin/bash",
		"--session-persist=true",
		"--session-dir", "/tmp/flag-sessions",
		"--session-buffer-lines", "900",
		"--max-watches", "200",
		"--temporal-dev-server",
		"--verbose",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != 7070 {
		t.Fatalf("expected port 7070, got %d", cfg.Port)
	}
	if cfg.Shell != "/bin/bash" {
		t.Fatalf("expected shell /bin/bash, got %q", cfg.Shell)
	}
	if !cfg.SessionPersist {
		t.Fatalf("expected session persistence enabled")
	}
	if cfg.SessionLogDir != "/tmp/flag-sessions" {
		t.Fatalf("expected session log dir /tmp/flag-sessions, got %q", cfg.SessionLogDir)
	}
	if cfg.SessionBufferLines != 900 {
		t.Fatalf("expected session buffer lines 900, got %d", cfg.SessionBufferLines)
	}
	if cfg.MaxWatches != 200 {
		t.Fatalf("expected max watches 200, got %d", cfg.MaxWatches)
	}
	if !cfg.TemporalDevServer {
		t.Fatalf("expected temporal dev server enabled")
	}
	if !cfg.Verbose {
		t.Fatalf("expected verbose true")
	}
	if cfg.Sources["port"] != sourceFlag {
		t.Fatalf("expected port source flag, got %q", cfg.Sources["port"])
	}
}

func TestLoadConfigInvalidFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "port", args: []string{"--port", "0"}},
		{name: "buffer", args: []string{"--session-buffer-lines", "0"}},
		{name: "retention", args: []string{"--session-retention-days", "0"}},
		{name: "max-watches", args: []string{"--max-watches", "0"}},
		{name: "shell-empty", args: []string{"--shell="}},
		{name: "session-dir-empty", args: []string{"--session-dir="}},
		{name: "input-history-dir-empty", args: []string{"--input-history-dir="}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := loadConfig(testCase.args); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestLoadConfigHelp(t *testing.T) {
	_, err := loadConfig([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
}

func TestLoadConfigHelpShort(t *testing.T) {
	_, err := loadConfig([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected ErrHelp, got %v", err)
	}
}

func TestLoadConfigVersion(t *testing.T) {
	cfg, err := loadConfig([]string{"--version"})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatalf("expected version flag to be set")
	}
}

func TestLoadConfigVersionShort(t *testing.T) {
	cfg, err := loadConfig([]string{"-v"})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.ShowVersion {
		t.Fatalf("expected version flag to be set")
	}
}
