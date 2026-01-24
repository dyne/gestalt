package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/client"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}
var startRetryDelay = time.Second
var agentCacheTTL = 60 * time.Second

type sendError struct {
	Code    int
	Message string
}

func (e *sendError) Error() string {
	return e.Message
}

func sendErr(code int, message string) *sendError {
	return &sendError{Code: code, Message: message}
}

func sendErrf(code int, format string, args ...any) *sendError {
	return &sendError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func sendErrFromClient(err error) *sendError {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return sendErr(3, httpErr.Message)
	}
	return sendErrf(3, "%v", err)
}

type agentInfo = client.AgentInfo

func handleSendError(err error, errOut io.Writer) int {
	var sendErr *sendError
	if errors.As(err, &sendErr) {
		fmt.Fprintln(errOut, sendErr.Message)
		if sendErr.Code != 0 {
			return sendErr.Code
		}
	}
	fmt.Fprintln(errOut, err.Error())
	return 3
}

func resolveAgent(cfg *Config) error {
	agents, err := loadAgents(*cfg)
	if err != nil {
		return sendErrf(3, "failed to fetch agents: %v", err)
	}
	if len(agents) == 0 {
		return sendErr(2, "no agents available")
	}

	input := strings.TrimSpace(cfg.AgentRef)
	if input == "" {
		return sendErr(2, "agent name or id required")
	}

	idMatches := make([]agentInfo, 0, 1)
	nameMatches := make([]agentInfo, 0, 1)
	for _, agent := range agents {
		if strings.EqualFold(agent.ID, input) {
			idMatches = append(idMatches, agent)
		}
		if strings.EqualFold(agent.Name, input) {
			nameMatches = append(nameMatches, agent)
		}
	}

	if len(idMatches) > 1 {
		return sendErrf(2, "input %q matches multiple agent ids: %s", input, formatAgentList(idMatches))
	}
	if len(nameMatches) > 1 {
		return sendErrf(2, "input %q matches multiple agent names: %s", input, formatAgentList(nameMatches))
	}

	var idMatch *agentInfo
	var nameMatch *agentInfo
	if len(idMatches) == 1 {
		idMatch = &idMatches[0]
	}
	if len(nameMatches) == 1 {
		nameMatch = &nameMatches[0]
	}

	if idMatch == nil && nameMatch == nil {
		return sendErrf(2, "agent %q not found", input)
	}

	if idMatch != nil && nameMatch != nil && idMatch.ID != nameMatch.ID {
		return sendErrf(2, "input %q matches agent id %q (name %q) and agent name %q (id %q)", input, idMatch.ID, idMatch.Name, nameMatch.Name, nameMatch.ID)
	}

	if idMatch != nil {
		cfg.AgentID = idMatch.ID
		cfg.AgentName = idMatch.Name
	} else if nameMatch != nil {
		cfg.AgentID = nameMatch.ID
		cfg.AgentName = nameMatch.Name
	}

	if cfg.Verbose {
		logf(*cfg, "resolved agent %q (id %q)", cfg.AgentName, cfg.AgentID)
	}

	return nil
}

func formatAgentList(agents []agentInfo) string {
	if len(agents) == 0 {
		return ""
	}
	entries := make([]string, 0, len(agents))
	for _, agent := range agents {
		entry := strings.TrimSpace(agent.ID)
		name := strings.TrimSpace(agent.Name)
		if name != "" {
			entry = entry + " (" + name + ")"
		}
		entries = append(entries, entry)
	}
	return strings.Join(entries, ", ")
}

func sendAgentInput(cfg Config, payload []byte) error {
	return sendAgentInputWithRetry(cfg, payload, true)
}

func sendAgentInputWithRetry(cfg Config, payload []byte, allowStart bool) error {
	if strings.TrimSpace(cfg.AgentName) == "" {
		return sendErr(2, "agent name not resolved")
	}
	baseURL := strings.TrimRight(cfg.URL, "/")
	if baseURL == "" {
		baseURL = defaultServerURL
	}
	target := fmt.Sprintf("%s/api/agents/%s/input", baseURL, cfg.AgentName)

	if cfg.Verbose {
		logf(cfg, "sending %d bytes to agent %q at %s", len(payload), cfg.AgentName, target)
		if strings.TrimSpace(cfg.Token) != "" {
			logf(cfg, "token: %s", maskToken(cfg.Token, cfg.Debug))
		}
	}
	if cfg.Debug && len(payload) > 0 {
		preview := payload
		if len(preview) > 100 {
			preview = preview[:100]
		}
		logf(cfg, "payload preview: %q", string(preview))
	}
	if err := client.SendAgentInput(httpClient, baseURL, cfg.Token, cfg.AgentName, payload); err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode == http.StatusNotFound && allowStart && cfg.Start {
				logf(cfg, "agent %q not running; attempting to start", cfg.AgentName)
				if err := client.StartAgent(httpClient, baseURL, cfg.Token, cfg.AgentID); err != nil {
					return sendErrFromClient(err)
				}
				time.Sleep(startRetryDelay)
				return sendAgentInputWithRetry(cfg, payload, false)
			}
			if cfg.Verbose && httpErr.StatusCode != 0 {
				logf(cfg, "response status: %d %s", httpErr.StatusCode, http.StatusText(httpErr.StatusCode))
			}
			if httpErr.StatusCode == http.StatusNotFound {
				return sendErr(2, httpErr.Message)
			}
			return sendErr(3, httpErr.Message)
		}
		return sendErrf(3, "%v", err)
	}
	if cfg.Verbose {
		logf(cfg, "response status: %d %s", http.StatusOK, http.StatusText(http.StatusOK))
	}
	return nil
}

func loadAgents(cfg Config) ([]agentInfo, error) {
	if agentCacheTTL > 0 {
		if cached, ok := readAgentCache(time.Now()); ok {
			return cached, nil
		}
	}

	baseURL := strings.TrimRight(cfg.URL, "/")
	if baseURL == "" {
		baseURL = defaultServerURL
	}
	agents, err := client.FetchAgents(httpClient, baseURL, cfg.Token)
	if err != nil {
		return nil, err
	}
	if agentCacheTTL > 0 {
		writeAgentCache(agents, time.Now())
	}
	return agents, nil
}

func fetchAgentNames(cfg Config) ([]string, error) {
	agents, err := loadAgents(cfg)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func readAgentCache(now time.Time) ([]agentInfo, bool) {
	path := agentCachePath()
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return nil, false
	}
	createdAt, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		return nil, false
	}
	if now.Sub(time.Unix(createdAt, 0)) > agentCacheTTL {
		return nil, false
	}
	agents := make([]agentInfo, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if id == "" || name == "" {
			continue
		}
		agents = append(agents, agentInfo{ID: id, Name: name})
	}
	if len(agents) == 0 {
		return nil, false
	}
	return agents, true
}

func writeAgentCache(agents []agentInfo, now time.Time) {
	if len(agents) == 0 {
		return
	}
	path := agentCachePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	var builder strings.Builder
	builder.WriteString(strconv.FormatInt(now.Unix(), 10))
	builder.WriteString("\n")
	for _, agent := range agents {
		if agent.ID == "" || agent.Name == "" {
			continue
		}
		builder.WriteString(agent.ID)
		builder.WriteString("\t")
		builder.WriteString(agent.Name)
		builder.WriteString("\n")
	}
	_ = os.WriteFile(path, []byte(builder.String()), 0o644)
}

func agentCachePath() string {
	cacheDir := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			cacheDir = filepath.Join(home, ".cache")
		}
	}
	if cacheDir == "" {
		return ""
	}
	return filepath.Join(cacheDir, "gestalt-send", "agents-cache.tsv")
}

func logf(cfg Config, format string, args ...any) {
	if cfg.LogWriter == nil || !(cfg.Verbose || cfg.Debug) {
		return
	}
	fmt.Fprintf(cfg.LogWriter, format+"\n", args...)
}

func maskToken(token string, debug bool) string {
	if debug {
		return token
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return trimmed[:2] + strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-2:]
}
