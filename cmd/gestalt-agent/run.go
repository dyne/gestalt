package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/runner/tmuxsession"
)

func runAgent(cfg Config, in io.Reader, out io.Writer, exec execRunner) (int, error) {
	baseURL := buildBaseURL(cfg.Host, cfg.Port)
	client := &http.Client{Timeout: 10 * time.Second}

	session, err := createExternalSession(client, baseURL, cfg.Token, cfg.AgentID)
	if err != nil {
		return exitServer, err
	}
	command, err := tmuxsession.AttachCommand(session.ID)
	if err != nil {
		return exitServer, err
	}
	if len(command) == 0 {
		return exitServer, fmt.Errorf("tmux attach command is empty")
	}
	if exec == nil {
		exec = runTmux
	}
	if command[0] == "tmux" {
		command = command[1:]
	}
	return exec(command)
}

func buildBaseURL(host string, port int) string {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		trimmedHost = defaultGestaltHost()
	}
	trimmedHost = strings.TrimPrefix(trimmedHost, "http://")
	trimmedHost = strings.TrimPrefix(trimmedHost, "https://")
	if port <= 0 {
		port = defaultGestaltPort()
	}
	return fmt.Sprintf("http://%s:%d", trimmedHost, port)
}
