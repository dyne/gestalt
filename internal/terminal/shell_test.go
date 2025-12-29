package terminal

import "testing"

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
			name:     "extra-whitespace",
			input:    "  /bin/bash   -l ",
			wantCmd:  "/bin/bash",
			wantArgs: []string{"-l"},
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
