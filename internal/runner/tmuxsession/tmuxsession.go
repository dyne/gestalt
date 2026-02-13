package tmuxsession

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/tmux"
)

// Client defines the tmux operations used by this package.
type Client interface {
	CreateSession(name string, command []string) error
	CreateWindow(sessionName, windowName string, command []string) error
	HasSession(name string) (bool, error)
}

// Target is the tmux target for a session window.
type Target struct {
	SessionName string
	WindowName  string
}

var newClient = func() Client {
	return tmux.NewClient()
}

var getwd = os.Getwd
var getenv = os.Getenv

// WorkdirSessionName returns the shared tmux session name for the current workdir.
func WorkdirSessionName() (string, error) {
	workdir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("get workdir: %w", err)
	}
	base := filepath.Base(workdir)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = "workspace"
	}
	return fmt.Sprintf("Gestalt %s", base), nil
}

// TargetForSession resolves tmux targeting for a session ID.
func TargetForSession(sessionID string) (Target, error) {
	windowName := sessionWindowName(sessionID)
	if insideTmux() {
		return Target{WindowName: windowName}, nil
	}
	sessionName, err := WorkdirSessionName()
	if err != nil {
		return Target{}, err
	}
	return Target{SessionName: sessionName, WindowName: windowName}, nil
}

// StartWindow ensures the workdir tmux session exists and creates the window for launch.
func StartWindow(launch *launchspec.LaunchSpec) error {
	if launch == nil {
		return errors.New("launch spec is required")
	}
	if len(launch.Argv) == 0 {
		return errors.New("launch argv is required")
	}
	client := newClient()
	if client == nil {
		return errors.New("tmux client unavailable")
	}
	target, err := TargetForSession(launch.SessionID)
	if err != nil {
		return err
	}
	if target.SessionName == "" {
		return client.CreateWindow("", target.WindowName, launch.Argv)
	}
	hasSession, err := client.HasSession(target.SessionName)
	if err != nil {
		return err
	}
	if !hasSession {
		if err := client.CreateSession(target.SessionName, nil); err != nil {
			return err
		}
	}
	return client.CreateWindow(target.SessionName, target.WindowName, launch.Argv)
}

// AttachCommand returns the tmux command to attach to or select a session window.
// When already inside tmux, it selects the session window if a session ID is provided.
func AttachCommand(sessionID string) ([]string, error) {
	sessionName, err := WorkdirSessionName()
	if err != nil {
		return nil, err
	}
	if insideTmux() {
		trimmed := strings.TrimSpace(sessionID)
		if trimmed == "" {
			return []string{"tmux", "switch-client", "-t", sessionName}, nil
		}
		return []string{"tmux", "select-window", "-t", sessionWindowName(trimmed)}, nil
	}
	return []string{"tmux", "attach", "-t", sessionName}, nil
}

func sessionWindowName(sessionID string) string {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return "gestalt-agent"
	}
	return trimmed
}

func insideTmux() bool {
	return strings.TrimSpace(getenv("TMUX")) != ""
}
