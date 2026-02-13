package tmuxsession

import (
	"errors"
	"testing"

	"gestalt/internal/runner/launchspec"
)

type fakeClient struct {
	createdSessions []string
	windows         []windowCall
	hasSession      map[string]bool
	hasSessionErr   error
}

type windowCall struct {
	sessionName string
	windowName  string
	command     []string
}

func (f *fakeClient) CreateSession(name string, _ []string) error {
	f.createdSessions = append(f.createdSessions, name)
	return nil
}

func (f *fakeClient) CreateWindow(sessionName, windowName string, command []string) error {
	f.windows = append(f.windows, windowCall{
		sessionName: sessionName,
		windowName:  windowName,
		command:     append([]string(nil), command...),
	})
	return nil
}

func (f *fakeClient) HasSession(name string) (bool, error) {
	if f.hasSessionErr != nil {
		return false, f.hasSessionErr
	}
	if f.hasSession == nil {
		return false, nil
	}
	return f.hasSession[name], nil
}

func TestWorkdirSessionNameFallback(t *testing.T) {
	originalGetwd := getwd
	getwd = func() (string, error) { return "/", nil }
	t.Cleanup(func() { getwd = originalGetwd })

	got, err := WorkdirSessionName()
	if err != nil {
		t.Fatalf("workdir session name: %v", err)
	}
	if got != "Gestalt workspace" {
		t.Fatalf("expected fallback session name, got %q", got)
	}
}

func TestTargetForSessionInsideTmux(t *testing.T) {
	originalGetenv := getenv
	getenv = func(key string) string {
		if key == "TMUX" {
			return "1"
		}
		return ""
	}
	t.Cleanup(func() { getenv = originalGetenv })

	target, err := TargetForSession("agent 1")
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if target.SessionName != "" {
		t.Fatalf("expected empty session name inside tmux, got %q", target.SessionName)
	}
	if target.WindowName != "agent 1" {
		t.Fatalf("expected window name %q, got %q", "agent 1", target.WindowName)
	}
}

func TestStartWindowInsideTmuxCreatesWindow(t *testing.T) {
	originalGetenv := getenv
	getenv = func(key string) string {
		if key == "TMUX" {
			return "1"
		}
		return ""
	}
	t.Cleanup(func() { getenv = originalGetenv })

	fake := &fakeClient{}
	originalClient := newClient
	newClient = func() Client { return fake }
	t.Cleanup(func() { newClient = originalClient })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := StartWindow(launch); err != nil {
		t.Fatalf("start window: %v", err)
	}
	if len(fake.createdSessions) != 0 {
		t.Fatalf("expected no sessions created, got %v", fake.createdSessions)
	}
	if len(fake.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(fake.windows))
	}
	if fake.windows[0].sessionName != "" {
		t.Fatalf("expected current session window, got %q", fake.windows[0].sessionName)
	}
}

func TestStartWindowUsesWorkdirSession(t *testing.T) {
	originalGetenv := getenv
	getenv = func(string) string { return "" }
	t.Cleanup(func() { getenv = originalGetenv })

	originalGetwd := getwd
	getwd = func() (string, error) { return "/tmp/repo", nil }
	t.Cleanup(func() { getwd = originalGetwd })

	fake := &fakeClient{
		hasSession: map[string]bool{"Gestalt repo": true},
	}
	originalClient := newClient
	newClient = func() Client { return fake }
	t.Cleanup(func() { newClient = originalClient })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := StartWindow(launch); err != nil {
		t.Fatalf("start window: %v", err)
	}
	if len(fake.createdSessions) != 0 {
		t.Fatalf("expected no sessions created, got %v", fake.createdSessions)
	}
	if len(fake.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(fake.windows))
	}
	if fake.windows[0].sessionName != "Gestalt repo" {
		t.Fatalf("expected session %q, got %q", "Gestalt repo", fake.windows[0].sessionName)
	}
}

func TestStartWindowCreatesSessionWhenMissing(t *testing.T) {
	originalGetenv := getenv
	getenv = func(string) string { return "" }
	t.Cleanup(func() { getenv = originalGetenv })

	originalGetwd := getwd
	getwd = func() (string, error) { return "/tmp/repo", nil }
	t.Cleanup(func() { getwd = originalGetwd })

	fake := &fakeClient{}
	originalClient := newClient
	newClient = func() Client { return fake }
	t.Cleanup(func() { newClient = originalClient })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := StartWindow(launch); err != nil {
		t.Fatalf("start window: %v", err)
	}
	if len(fake.createdSessions) != 1 || fake.createdSessions[0] != "Gestalt repo" {
		t.Fatalf("expected session created, got %v", fake.createdSessions)
	}
}

func TestAttachCommandOutsideTmux(t *testing.T) {
	originalGetenv := getenv
	getenv = func(string) string { return "" }
	t.Cleanup(func() { getenv = originalGetenv })

	originalGetwd := getwd
	getwd = func() (string, error) { return "/tmp/repo", nil }
	t.Cleanup(func() { getwd = originalGetwd })

	cmd, err := AttachCommand("agent 1")
	if err != nil {
		t.Fatalf("attach command: %v", err)
	}
	want := []string{"tmux", "attach", "-t", "Gestalt repo"}
	if len(cmd) != len(want) {
		t.Fatalf("unexpected command: %v", cmd)
	}
	for i := range want {
		if cmd[i] != want[i] {
			t.Fatalf("unexpected command[%d]: got %q want %q", i, cmd[i], want[i])
		}
	}
}

func TestAttachCommandInsideTmux(t *testing.T) {
	originalGetenv := getenv
	getenv = func(key string) string {
		if key == "TMUX" {
			return "1"
		}
		return ""
	}
	t.Cleanup(func() { getenv = originalGetenv })

	originalGetwd := getwd
	getwd = func() (string, error) { return "/tmp/repo", nil }
	t.Cleanup(func() { getwd = originalGetwd })

	cmd, err := AttachCommand("agent 1")
	if err != nil {
		t.Fatalf("attach command: %v", err)
	}
	want := []string{"tmux", "select-window", "-t", "agent 1"}
	if len(cmd) != len(want) {
		t.Fatalf("unexpected command: %v", cmd)
	}
	for i := range want {
		if cmd[i] != want[i] {
			t.Fatalf("unexpected command[%d]: got %q want %q", i, cmd[i], want[i])
		}
	}
}

func TestWorkdirSessionNameGetwdError(t *testing.T) {
	originalGetwd := getwd
	getwd = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { getwd = originalGetwd })

	if _, err := WorkdirSessionName(); err == nil {
		t.Fatal("expected error")
	}
}
