package main

import (
	"errors"
	"strings"
	"unicode"

	"gestalt/internal/runner/launchspec"
	"gestalt/internal/runner/tmux"
)

type tmuxStarter interface {
	CreateSession(name string, command []string) error
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
	sessionName := tmuxSessionName(launch.SessionID)
	return client.CreateSession(sessionName, launch.Argv)
}

func tmuxSessionName(sessionID string) string {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		trimmed = "gestalt-agent"
	}
	var builder strings.Builder
	builder.Grow(len(trimmed))
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('_')
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "gestalt-agent"
	}
	return result
}
