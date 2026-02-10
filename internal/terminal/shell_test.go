package terminal

import (
	"strings"
	"testing"
)

func TestDefaultShellFor(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "windows-comspec",
			goos: "windows",
			env:  map[string]string{"ComSpec": "C:\\Windows\\System32\\cmd.exe"},
			want: "C:\\Windows\\System32\\cmd.exe",
		},
		{
			name: "windows-comspec-uppercase",
			goos: "windows",
			env:  map[string]string{"COMSPEC": "C:\\Windows\\System32\\cmd.exe"},
			want: "C:\\Windows\\System32\\cmd.exe",
		},
		{
			name: "windows-default",
			goos: "windows",
			env:  map[string]string{},
			want: "cmd.exe",
		},
		{
			name: "unix-shell",
			goos: "linux",
			env:  map[string]string{"SHELL": "/bin/zsh"},
			want: "/bin/zsh",
		},
		{
			name: "unix-default",
			goos: "linux",
			env:  map[string]string{},
			want: "/bin/bash",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := defaultShellFor(test.goos, func(key string) string {
				return test.env[key]
			})
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCmd   string
		wantArgs  []string
		wantError bool
	}{
		{
			name:     "single-command",
			input:    "/bin/bash",
			wantCmd:  "/bin/bash",
			wantArgs: nil,
		},
		{
			name:     "command-with-args",
			input:    "copilot --allow-all-tools --disable-builtin-mcps",
			wantCmd:  "copilot",
			wantArgs: []string{"--allow-all-tools", "--disable-builtin-mcps"},
		},
		{
			name:     "single-quoted-arg",
			input:    "codex -c 'approval_policy=never'",
			wantCmd:  "codex",
			wantArgs: []string{"-c", "approval_policy=never"},
		},
		{
			name:     "double-quoted-arg",
			input:    "codex -c \"compact_prompt=Study your current L1\"",
			wantCmd:  "codex",
			wantArgs: []string{"-c", "compact_prompt=Study your current L1"},
		},
		{
			name:     "mixed-quote-concat",
			input:    "codex -c 'instructions=fix '\"'\"'this'\"'\"' now'",
			wantCmd:  "codex",
			wantArgs: []string{"-c", "instructions=fix 'this' now"},
		},
		{
			name:     "extra-whitespace",
			input:    "  /bin/bash   -l ",
			wantCmd:  "/bin/bash",
			wantArgs: []string{"-l"},
		},
		{
			name:     "backslash-escape",
			input:    "cmd path\\ with\\ spaces",
			wantCmd:  "cmd",
			wantArgs: []string{"path with spaces"},
		},
		{
			name:      "empty",
			input:     "   ",
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd, args, err := splitCommandLine(test.input)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != test.wantCmd {
				t.Fatalf("expected command %q, got %q", test.wantCmd, cmd)
			}
			if len(args) != len(test.wantArgs) {
				t.Fatalf("expected args %v, got %v", test.wantArgs, args)
			}
			for i := range args {
				if args[i] != test.wantArgs[i] {
					t.Fatalf("expected args %v, got %v", test.wantArgs, args)
				}
			}
		})
	}
}

func TestRedactDeveloperInstructionsShell(t *testing.T) {
	input := "codex -c 'developer_instructions=hello world' -c approval_policy=never"
	output := redactDeveloperInstructionsShell(input)
	if strings.Contains(output, "hello world") {
		t.Fatalf("expected developer instructions to be redacted, got %q", output)
	}
	if !strings.Contains(output, "developer_instructions=<skip>") {
		t.Fatalf("expected redaction marker, got %q", output)
	}
	if !strings.Contains(output, "approval_policy=never") {
		t.Fatalf("expected other args to remain, got %q", output)
	}
}

func TestRedactDeveloperInstructionsShellFallback(t *testing.T) {
	input := "codex -c 'developer_instructions=unterminated"
	output := redactDeveloperInstructionsShell(input)
	if !strings.Contains(output, "developer_instructions=<skip>") {
		t.Fatalf("expected redaction marker, got %q", output)
	}
}
