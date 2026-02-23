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
