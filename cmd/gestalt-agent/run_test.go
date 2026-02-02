package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsageExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithExec([]string{}, &stdout, &stderr, nil)
	if code != exitUsage {
		t.Fatalf("expected exit code %d, got %d", exitUsage, code)
	}
	if !strings.Contains(stderr.String(), "agent id required") {
		t.Fatalf("expected error message, got %q", stderr.String())
	}
}

func TestRunAgentExitCodes(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		agentID     string
		wantCode    int
		wantSubstrs []string
	}{
		{
			name: "config path is file",
			setup: func(t *testing.T, dir string) {
				path := filepath.Join(dir, defaultConfigDir)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir config parent: %v", err)
				}
				if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
					t.Fatalf("write config file: %v", err)
				}
			},
			agentID:  "coder",
			wantCode: exitConfig,
			wantSubstrs: []string{
				defaultConfigDir,
			},
		},
		{
			name:     "agent missing",
			setup:    func(t *testing.T, dir string) {},
			agentID:  "missing-agent",
			wantCode: exitAgent,
			wantSubstrs: []string{
				localAgentPath("missing-agent"),
				fallbackAgentPath("missing-agent"),
			},
		},
		{
			name: "prompt missing",
			setup: func(t *testing.T, dir string) {
				promptDir := filepath.Join(dir, "config", "prompts")
				agentDir := filepath.Join(dir, "config", "agents")
				if err := os.MkdirAll(promptDir, 0o755); err != nil {
					t.Fatalf("mkdir prompts: %v", err)
				}
				if err := os.MkdirAll(agentDir, 0o755); err != nil {
					t.Fatalf("mkdir agents: %v", err)
				}
				agentConfig := "name=\"Test\"\ncli_type=\"codex\"\nprompt=\"missing-prompt\"\n[cli_config]\nmodel=\"gpt-4\"\n"
				if err := os.WriteFile(filepath.Join(agentDir, "test.toml"), []byte(agentConfig), 0o644); err != nil {
					t.Fatalf("write agent: %v", err)
				}
			},
			agentID:  "test",
			wantCode: exitPrompt,
			wantSubstrs: []string{
				localPromptsDir(),
				fallbackPromptsDir(),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workdir := t.TempDir()
			withWorkdir(t, workdir, func() {
				if tc.setup != nil {
					tc.setup(t, workdir)
				}
				cfg := Config{AgentID: tc.agentID}
				code, err := runAgent(cfg, bytes.NewReader(nil), bytes.NewBuffer(nil), nil)
				if code != tc.wantCode {
					t.Fatalf("expected exit code %d, got %d", tc.wantCode, code)
				}
				if err == nil {
					t.Fatalf("expected error")
				}
				for _, substr := range tc.wantSubstrs {
					if !strings.Contains(err.Error(), substr) {
						t.Fatalf("expected error to contain %q, got %q", substr, err.Error())
					}
				}
			})
		})
	}
}

func TestRunDryRunPrintsCommand(t *testing.T) {
	workdir := t.TempDir()
	withWorkdir(t, workdir, func() {
		agentDir := filepath.Join(workdir, "config", "agents")
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatalf("mkdir agents: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(workdir, ".gestalt", "config"), 0o755); err != nil {
			t.Fatalf("mkdir fallback config: %v", err)
		}
		agentConfig := "name=\"DryRun\"\ncli_type=\"codex\"\n[cli_config]\nmodel=\"gpt-4\"\n"
		if err := os.WriteFile(filepath.Join(agentDir, "dry.toml"), []byte(agentConfig), 0o644); err != nil {
			t.Fatalf("write agent: %v", err)
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := runWithExec([]string{"--dryrun", "dry"}, &stdout, &stderr, func(args []string) (int, error) {
			return 0, nil
		})
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, stderr.String())
		}
		output := stdout.String()
		if !strings.Contains(output, "codex") {
			t.Fatalf("expected command output, got %q", output)
		}
		if !strings.Contains(output, `developer_instructions=""`) {
			t.Fatalf("expected developer_instructions in output, got %q", output)
		}
	})
}
