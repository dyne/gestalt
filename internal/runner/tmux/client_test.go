package tmux

import (
	"bytes"
	"testing"
)

type tmuxCall struct {
	args  []string
	input []byte
}

type fakeRunner struct {
	calls  []tmuxCall
	output []byte
	err    error
}

func (f *fakeRunner) Run(args []string, input []byte) ([]byte, error) {
	f.calls = append(f.calls, tmuxCall{args: append([]string(nil), args...), input: append([]byte(nil), input...)})
	return f.output, f.err
}

func TestClientCreateSession(t *testing.T) {
	runner := &fakeRunner{}
	client := NewClientWithRunner(runner)

	if err := client.CreateSession("sess", []string{"bash", "-lc", "echo hi"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	got := runner.calls[0].args
	expected := []string{"new-session", "-d", "-s", "sess", "--", "bash", "-lc", "echo hi"}
	if !equalArgs(got, expected) {
		t.Fatalf("unexpected args: %#v", got)
	}
}

func TestClientLoadBuffer(t *testing.T) {
	runner := &fakeRunner{}
	client := NewClientWithRunner(runner)

	payload := []byte("hello")
	if err := client.LoadBuffer(payload); err != nil {
		t.Fatalf("load buffer: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	expected := []string{"load-buffer", "-"}
	if !equalArgs(call.args, expected) {
		t.Fatalf("unexpected args: %#v", call.args)
	}
	if !bytes.Equal(call.input, payload) {
		t.Fatalf("unexpected input: %q", call.input)
	}
}

func TestClientCapturePane(t *testing.T) {
	runner := &fakeRunner{output: []byte("captured")}
	client := NewClientWithRunner(runner)

	output, err := client.CapturePane("sess:0.0")
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if string(output) != "captured" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestClientResizePane(t *testing.T) {
	runner := &fakeRunner{}
	client := NewClientWithRunner(runner)

	if err := client.ResizePane("sess:0.0", 80, 24); err != nil {
		t.Fatalf("resize pane: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	expected := []string{"resize-pane", "-t", "sess:0.0", "-x", "80", "-y", "24"}
	if !equalArgs(runner.calls[0].args, expected) {
		t.Fatalf("unexpected args: %#v", runner.calls[0].args)
	}
}

func equalArgs(got, expected []string) bool {
	if len(got) != len(expected) {
		return false
	}
	for i := range expected {
		if got[i] != expected[i] {
			return false
		}
	}
	return true
}
