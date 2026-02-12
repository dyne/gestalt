package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/tmux"
)

type tmuxStarter interface {
	CreateSession(name string, command []string) error
	CreateWindow(sessionName, windowName string, command []string) error
	HasSession(name string) (bool, error)
}

var tmuxClientFactory = func() tmuxStarter {
	return tmux.NewClient()
}

func startTmuxSession(launch *launchspec.LaunchSpec) error {
	if launch == nil {
		return errors.New("launch spec is required")
	}
	if len(launch.Argv) == 0 {
		return errors.New("launch argv is required")
	}
	client := tmuxClientFactory()
	if client == nil {
		return errors.New("tmux client unavailable")
	}
	target, err := tmuxTargetForSession(launch.SessionID)
	if err != nil {
		return err
	}
	if target.sessionName == "" {
		return client.CreateWindow("", target.windowName, launch.Argv)
	}
	hasSession, err := client.HasSession(target.sessionName)
	if err != nil {
		return err
	}
	if !hasSession {
		if err := client.CreateSession(target.sessionName, nil); err != nil {
			return err
		}
	}
	return client.CreateWindow(target.sessionName, target.windowName, launch.Argv)
}

func tmuxSessionName(sessionID string) string {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		trimmed = "gestalt-agent"
	}
	return trimmed
}

type tmuxTarget struct {
	sessionName string
	windowName  string
}

// PaneTarget returns the tmux pane target for the session/window.
func (t tmuxTarget) PaneTarget() string {
	if t.sessionName == "" {
		return t.windowName
	}
	return fmt.Sprintf("%s:%s", t.sessionName, t.windowName)
}

// tmuxTargetForSession resolves the tmux session/window targeting rules.
func tmuxTargetForSession(sessionID string) (tmuxTarget, error) {
	windowName := tmuxSessionName(sessionID)
	if tmuxInSession() {
		return tmuxTarget{windowName: windowName}, nil
	}
	sessionName, err := tmuxWorkdirSessionName()
	if err != nil {
		return tmuxTarget{}, err
	}
	return tmuxTarget{sessionName: sessionName, windowName: windowName}, nil
}

// tmuxInSession reports whether the process is running inside tmux.
func tmuxInSession() bool {
	return strings.TrimSpace(os.Getenv("TMUX")) != ""
}

// tmuxWorkdirSessionName builds the shared tmux session name for the current workdir.
func tmuxWorkdirSessionName() (string, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get workdir: %w", err)
	}
	base := filepath.Base(workdir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = "workspace"
	}
	return fmt.Sprintf("Gestalt %s", base), nil
}
