package client

import "testing"

func TestNormalizeSessionRef(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "explicit name preserved", input: "Coder", want: "Coder"},
		{name: "trimmed explicit name", input: "  Coder  ", want: "Coder"},
		{name: "explicit numbered", input: "Coder 2", want: "Coder 2"},
		{name: "explicit numbered extra spaces", input: "Coder   15", want: "Coder   15"},
		{name: "custom id preserved", input: "tmux-hub", want: "tmux-hub"},
		{name: "numeric id preserved", input: "1", want: "1"},
	}

	for _, tc := range cases {
		got, err := NormalizeSessionRef(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestResolveSessionRefAgainstSessions(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "Fixer 1"},
		{ID: "tmux-hub"},
	}
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "name resolves to canonical running", input: "Fixer", want: "Fixer 1"},
		{name: "explicit numbered preserved", input: "Fixer 1", want: "Fixer 1"},
		{name: "raw system id preserved when running", input: "tmux-hub", want: "tmux-hub"},
		{name: "missing name uses canonical fallback", input: "Builder", want: "Builder 1"},
	}

	for _, tc := range cases {
		got, err := ResolveSessionRefAgainstSessions(tc.input, sessions)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestIsExplicitNumberedSessionRef(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{ref: "Fixer", want: false},
		{ref: "Fixer 1", want: true},
		{ref: "Fixer 20", want: true},
		{ref: "tmux-hub", want: false},
		{ref: "123", want: false},
	}
	for _, tc := range cases {
		if got := IsExplicitNumberedSessionRef(tc.ref); got != tc.want {
			t.Fatalf("ref %q: expected %v, got %v", tc.ref, tc.want, got)
		}
	}
}

func TestIsChatSessionRef(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{ref: "chat", want: true},
		{ref: " Chat ", want: true},
		{ref: "CHAT", want: true},
		{ref: "chat 1", want: false},
		{ref: "Coder", want: false},
	}
	for _, tc := range cases {
		if got := IsChatSessionRef(tc.ref); got != tc.want {
			t.Fatalf("ref %q: expected %v, got %v", tc.ref, tc.want, got)
		}
	}
}
