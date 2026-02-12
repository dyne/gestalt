package main

import (
	"testing"

	"gestalt/internal/runner/launchspec"
)

type fakeTmuxStarter struct {
	createdSessions []string
	windows         []tmuxWindowCall
	hasSession      map[string]bool
	hasSessionErr   error
}

type tmuxWindowCall struct {
	sessionName string
	windowName  string
	command     []string
}

func (f *fakeTmuxStarter) CreateSession(name string, command []string) error {
	f.createdSessions = append(f.createdSessions, name)
	return nil
}

func (f *fakeTmuxStarter) CreateWindow(sessionName, windowName string, command []string) error {
	f.windows = append(f.windows, tmuxWindowCall{
		sessionName: sessionName,
		windowName:  windowName,
		command:     append([]string(nil), command...),
	})
	return nil
}

func (f *fakeTmuxStarter) HasSession(name string) (bool, error) {
	if f.hasSessionErr != nil {
		return false, f.hasSessionErr
	}
	if f.hasSession == nil {
		return false, nil
	}
	return f.hasSession[name], nil
}

func TestStartTmuxSessionInsideTmuxCreatesWindow(t *testing.T) {
	t.Setenv("TMUX", "1")
	fake := &fakeTmuxStarter{}
	original := tmuxClientFactory
	tmuxClientFactory = func() tmuxStarter { return fake }
	t.Cleanup(func() { tmuxClientFactory = original })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := startTmuxSession(launch); err != nil {
		t.Fatalf("start tmux session: %v", err)
	}
	if len(fake.createdSessions) != 0 {
		t.Fatalf("expected no sessions created, got %v", fake.createdSessions)
	}
	if len(fake.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(fake.windows))
	}
	if fake.windows[0].sessionName != "" {
		t.Fatalf("expected current-session window, got %q", fake.windows[0].sessionName)
	}
	if fake.windows[0].windowName != "agent 1" {
		t.Fatalf("expected window name %q, got %q", "agent 1", fake.windows[0].windowName)
	}
	if len(fake.windows[0].command) != len(launch.Argv) {
		t.Fatalf("expected command %v, got %v", launch.Argv, fake.windows[0].command)
	}
}

func TestStartTmuxSessionUsesWorkdirSession(t *testing.T) {
	t.Setenv("TMUX", "")
	sessionName, err := tmuxWorkdirSessionName()
	if err != nil {
		t.Fatalf("workdir session name: %v", err)
	}
	fake := &fakeTmuxStarter{
		hasSession: map[string]bool{sessionName: true},
	}
	original := tmuxClientFactory
	tmuxClientFactory = func() tmuxStarter { return fake }
	t.Cleanup(func() { tmuxClientFactory = original })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := startTmuxSession(launch); err != nil {
		t.Fatalf("start tmux session: %v", err)
	}
	if len(fake.createdSessions) != 0 {
		t.Fatalf("expected no sessions created, got %v", fake.createdSessions)
	}
	if len(fake.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(fake.windows))
	}
	if fake.windows[0].sessionName != sessionName {
		t.Fatalf("expected window session %q, got %q", sessionName, fake.windows[0].sessionName)
	}
	if fake.windows[0].windowName != "agent 1" {
		t.Fatalf("expected window name %q, got %q", "agent 1", fake.windows[0].windowName)
	}
}

func TestStartTmuxSessionCreatesSessionIfMissing(t *testing.T) {
	t.Setenv("TMUX", "")
	sessionName, err := tmuxWorkdirSessionName()
	if err != nil {
		t.Fatalf("workdir session name: %v", err)
	}
	fake := &fakeTmuxStarter{}
	original := tmuxClientFactory
	tmuxClientFactory = func() tmuxStarter { return fake }
	t.Cleanup(func() { tmuxClientFactory = original })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent 1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := startTmuxSession(launch); err != nil {
		t.Fatalf("start tmux session: %v", err)
	}
	if len(fake.createdSessions) != 1 || fake.createdSessions[0] != sessionName {
		t.Fatalf("expected session %q created, got %v", sessionName, fake.createdSessions)
	}
	if len(fake.windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(fake.windows))
	}
}

func TestTmuxSessionNameFallback(t *testing.T) {
	if got := tmuxSessionName("   "); got == "" {
		t.Fatalf("expected fallback name, got empty")
	}
}
