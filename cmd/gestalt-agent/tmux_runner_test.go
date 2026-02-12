package main

import (
	"testing"

	"gestalt/internal/runner/launchspec"
)

type fakeTmuxStarter struct {
	name    string
	command []string
}

func (f *fakeTmuxStarter) CreateSession(name string, command []string) error {
	f.name = name
	f.command = append([]string(nil), command...)
	return nil
}

func TestStartTmuxSessionUsesSanitizedName(t *testing.T) {
	fake := &fakeTmuxStarter{}
	original := tmuxClientFactory
	tmuxClientFactory = func() tmuxStarter { return fake }
	t.Cleanup(func() { tmuxClientFactory = original })

	launch := &launchspec.LaunchSpec{
		SessionID: "agent/1",
		Argv:      []string{"codex", "-c", "model=o3"},
	}
	if err := startTmuxSession(launch); err != nil {
		t.Fatalf("start tmux session: %v", err)
	}
	if fake.name != "agent_1" {
		t.Fatalf("expected sanitized name, got %q", fake.name)
	}
	if len(fake.command) != len(launch.Argv) {
		t.Fatalf("expected command %v, got %v", launch.Argv, fake.command)
	}
}

func TestTmuxSessionNameFallback(t *testing.T) {
	if got := tmuxSessionName("   "); got == "" {
		t.Fatalf("expected fallback name, got empty")
	}
}
