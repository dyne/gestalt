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
