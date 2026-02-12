package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func runAgent(cfg Config, in io.Reader, out io.Writer, exec execRunner) (int, error) {
	baseURL := strings.TrimSpace(cfg.URL)
	if baseURL == "" {
		baseURL = defaultGestaltURL()
	}
	client := &http.Client{Timeout: 10 * time.Second}

	session, err := createExternalSession(client, baseURL, cfg.Token, cfg.AgentID)
	if err != nil {
		return exitServer, err
	}
	if session.Launch == nil {
		return exitServer, errors.New("launch spec missing from server response")
	}
	argv := session.Launch.Argv
	if len(argv) == 0 {
		return exitServer, errors.New("launch argv missing from server response")
	}
	command := argv[0]
	args := argv[1:]
	if !strings.EqualFold(command, "codex") {
		return exitServer, fmt.Errorf("unsupported launch command %q", command)
	}
	if exec == nil {
		if err := startTmuxSession(session.Launch); err != nil {
			return exitServer, err
		}
		return 0, nil
	}
	return exec(args)
}
