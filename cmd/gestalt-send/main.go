package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/version"
)

const defaultServerURL = "http://localhost:8080"

var httpClient = &http.Client{Timeout: 30 * time.Second}
var startRetryDelay = time.Second
var agentCacheTTL = 60 * time.Second

type Config struct {
	URL         string
	Token       string
	AgentRef    string
	AgentID     string
	AgentName   string
	Start       bool
	Verbose     bool
	Debug       bool
	ShowVersion bool
	LogWriter   io.Writer
}

type sendError struct {
	Code    int
	Message string
}

func (e *sendError) Error() string {
	return e.Message
}

type agentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "completion":
			os.Exit(runCompletion(os.Args[2:], os.Stdout, os.Stderr))
		case "__complete-agents":
			os.Exit(runCompleteAgents(os.Args[2:], os.Stdout, os.Stderr))
		}
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

func run(args []string, in io.Reader, errOut io.Writer) int {
	return runWithSender(args, in, errOut, sendAgentInput)
}

func runWithSender(args []string, in io.Reader, errOut io.Writer, send func(Config, []byte) error) int {
	cfg, err := parseArgs(args, errOut)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(os.Stdout, "gestalt-send dev")
		} else {
			fmt.Fprintf(os.Stdout, "gestalt-send version %s\n", version.Version)
		}
		return 0
	}
	if cfg.Debug {
		cfg.Verbose = true
	}
	cfg.LogWriter = errOut

	if err := resolveAgent(&cfg); err != nil {
		return handleSendError(err, errOut)
	}

	payload, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintf(errOut, "read stdin: %v\n", err)
		return 3
	}

	if send == nil {
		return 0
	}
	if err := send(cfg, payload); err != nil {
		return handleSendError(err, errOut)
	}
	return 0
}

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-send", flag.ContinueOnError)
	fs.SetOutput(errOut)
	urlFlag := fs.String("url", "", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:8080)")
	tokenFlag := fs.String("token", "", "Auth token (env: GESTALT_TOKEN, default: none)")
	startFlag := fs.Bool("start", false, "Start agent if not running")
	verboseFlag := fs.Bool("verbose", false, "Verbose output")
	debugFlag := fs.Bool("debug", false, "Debug output (implies --verbose)")
	helpFlag := fs.Bool("help", false, "Show this help message")
	versionFlag := fs.Bool("version", false, "Print version and exit")
	fs.Usage = func() {
		printSendHelp(fs.Output())
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if *helpFlag {
		fs.Usage()
		return Config{}, flag.ErrHelp
	}

	if *versionFlag {
		return Config{ShowVersion: true}, nil
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return Config{}, fmt.Errorf("agent name or id required")
	}

	agentRef := strings.TrimSpace(fs.Arg(0))
	if agentRef == "" {
		fs.Usage()
		return Config{}, fmt.Errorf("agent name or id required")
	}

	url := strings.TrimSpace(*urlFlag)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("GESTALT_URL"))
	}
	if url == "" {
		url = defaultServerURL
	}

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
	}

	return Config{
		URL:      url,
		Token:    token,
		AgentRef: agentRef,
		Start:    *startFlag,
		Verbose:  *verboseFlag,
		Debug:    *debugFlag,
	}, nil
}

func printSendHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt-send [options] <agent-name-or-id>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Send stdin to a running Gestalt agent terminal")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeSendOption(out, "--url URL", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:8080)")
	writeSendOption(out, "--token TOKEN", "Auth token (env: GESTALT_TOKEN, default: none)")
	writeSendOption(out, "--start", "Auto-start agent if not running")
	writeSendOption(out, "--verbose", "Show request/response details")
	writeSendOption(out, "--debug", "Show detailed debug info (implies --verbose)")
	writeSendOption(out, "--help", "Show this help message")
	writeSendOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Arguments:")
	fmt.Fprintln(out, "  agent-name-or-id  Agent name or ID to send input to")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  cat file.txt | gestalt-send copilot")
	fmt.Fprintln(out, "  echo \"status\" | gestalt-send --start architect")
	fmt.Fprintln(out, "  gestalt-send --url http://remote:8080 --token abc123 agent-id")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Exit codes:")
	fmt.Fprintln(out, "  0  Success")
	fmt.Fprintln(out, "  1  Usage error")
	fmt.Fprintln(out, "  2  Agent not running")
	fmt.Fprintln(out, "  3  Network or server error")
}

func writeSendOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-14s %s\n", name, desc)
}

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
	agents, err := fetchAgents(*cfg)
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("failed to fetch agents: %v", err)}
	}
	if len(agents) == 0 {
		return &sendError{Code: 2, Message: "no agents available"}
	}

	input := strings.TrimSpace(cfg.AgentRef)
	if input == "" {
		return &sendError{Code: 2, Message: "agent name or id required"}
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
		return &sendError{Code: 2, Message: fmt.Sprintf("input %q matches multiple agent ids: %s", input, formatAgentList(idMatches))}
	}
	if len(nameMatches) > 1 {
		return &sendError{Code: 2, Message: fmt.Sprintf("input %q matches multiple agent names: %s", input, formatAgentList(nameMatches))}
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
		return &sendError{Code: 2, Message: fmt.Sprintf("agent %q not found", input)}
	}

	if idMatch != nil && nameMatch != nil && idMatch.ID != nameMatch.ID {
		return &sendError{Code: 2, Message: fmt.Sprintf("input %q matches agent id %q (name %q) and agent name %q (id %q)", input, idMatch.ID, idMatch.Name, nameMatch.Name, nameMatch.ID)}
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
		return &sendError{Code: 2, Message: "agent name not resolved"}
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

	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("build request failed: %v", err)}
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	if strings.TrimSpace(cfg.Token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("request failed: %v", err)}
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		message := parseErrorMessage(body)
		if message == "" {
			message = response.Status
		}
		if response.StatusCode == http.StatusNotFound && allowStart && cfg.Start {
			logf(cfg, "agent %q not running; attempting to start", cfg.AgentName)
			if err := startAgent(cfg, baseURL); err != nil {
				return err
			}
			time.Sleep(startRetryDelay)
			return sendAgentInputWithRetry(cfg, payload, false)
		}
		if cfg.Verbose {
			logf(cfg, "response status: %s", response.Status)
		}
		if response.StatusCode == http.StatusNotFound {
			return &sendError{Code: 2, Message: message}
		}
		return &sendError{Code: 3, Message: message}
	}

	if cfg.Verbose {
		logf(cfg, "response status: %s", response.Status)
	}
	return nil
}

func parseErrorMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if strings.TrimSpace(payload.Error) != "" {
			return payload.Error
		}
	}
	return text
}

func startAgent(cfg Config, baseURL string) error {
	agentID := strings.TrimSpace(cfg.AgentID)
	if agentID == "" {
		return &sendError{Code: 2, Message: "agent id not resolved"}
	}
	payload := map[string]string{"agent": agentID}
	body, err := json.Marshal(payload)
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("encode start request: %v", err)}
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/terminals", bytes.NewReader(body))
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("build start request: %v", err)}
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.Token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return &sendError{Code: 3, Message: fmt.Sprintf("start request failed: %v", err)}
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusOK || response.StatusCode == http.StatusConflict {
		if cfg.Verbose {
			logf(cfg, "agent start response: %s", response.Status)
		}
		return nil
	}

	respBody, _ := io.ReadAll(response.Body)
	message := parseErrorMessage(respBody)
	if message == "" {
		message = response.Status
	}
	return &sendError{Code: 3, Message: message}
}

func runCompletion(args []string, out io.Writer, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: gestalt-send completion [bash|zsh]")
		return 1
	}
	switch args[0] {
	case "bash":
		_, _ = io.WriteString(out, bashCompletionScript)
		return 0
	case "zsh":
		_, _ = io.WriteString(out, zshCompletionScript)
		return 0
	default:
		fmt.Fprintln(errOut, "usage: gestalt-send completion [bash|zsh]")
		return 1
	}
}

func runCompleteAgents(args []string, out io.Writer, errOut io.Writer) int {
	cfg, err := parseCompletionArgs(args, errOut)
	if err != nil {
		return 1
	}
	names, err := fetchAgentNames(cfg)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 3
	}
	if len(names) == 0 {
		return 0
	}
	fmt.Fprint(out, strings.Join(names, " "))
	return 0
}

func parseCompletionArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-send", flag.ContinueOnError)
	fs.SetOutput(errOut)
	urlFlag := fs.String("url", "", "Gestalt server URL")
	tokenFlag := fs.String("token", "", "Gestalt auth token")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "usage: gestalt-send __complete-agents [--url URL] [--token TOKEN]")
	}
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	url := strings.TrimSpace(*urlFlag)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("GESTALT_URL"))
	}
	if url == "" {
		url = defaultServerURL
	}

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
	}

	return Config{
		URL:   url,
		Token: token,
	}, nil
}

func fetchAgents(cfg Config) ([]agentInfo, error) {
	if agentCacheTTL > 0 {
		if cached, ok := readAgentCache(time.Now()); ok {
			return cached, nil
		}
	}

	baseURL := strings.TrimRight(cfg.URL, "/")
	if baseURL == "" {
		baseURL = defaultServerURL
	}
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/agents", nil)
	if err != nil {
		return nil, fmt.Errorf("build agents request failed: %w", err)
	}
	if strings.TrimSpace(cfg.Token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("agents request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		message := parseErrorMessage(body)
		if message == "" {
			message = response.Status
		}
		return nil, errors.New(message)
	}

	var payload []agentInfo
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode agents response: %w", err)
	}
	agents := make([]agentInfo, 0, len(payload))
	for _, agent := range payload {
		id := strings.TrimSpace(agent.ID)
		name := strings.TrimSpace(agent.Name)
		if id == "" || name == "" {
			continue
		}
		agents = append(agents, agentInfo{ID: id, Name: name})
	}
	if agentCacheTTL > 0 {
		writeAgentCache(agents, time.Now())
	}
	return agents, nil
}

func fetchAgentNames(cfg Config) ([]string, error) {
	agents, err := fetchAgents(cfg)
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
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
	_ = os.WriteFile(path, []byte(builder.String()), 0644)
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

const bashCompletionScript = `# Bash completion for gestalt-send
_gestalt_send_cached_agents() {
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/gestalt-send"
  local cache_file="$cache_dir/agents"
  local now
  now=$(date +%s)
  local cached=""

  if [[ -f "$cache_file" ]]; then
    local ts
    ts=$(head -n 1 "$cache_file" 2>/dev/null)
    if [[ "$ts" =~ ^[0-9]+$ ]]; then
      local age=$((now - ts))
      if (( age < 60 )); then
        cached=$(tail -n +2 "$cache_file" 2>/dev/null)
      fi
    fi
  fi

  if [[ -z "$cached" ]]; then
    cached=$(gestalt-send __complete-agents 2>/dev/null)
    if [[ -n "$cached" ]]; then
      mkdir -p "$cache_dir" 2>/dev/null
      { echo "$now"; echo "$cached"; } > "$cache_file" 2>/dev/null
    fi
  fi

  echo "$cached"
}

_gestalt_send_complete() {
  local cur prev
  _get_comp_words_by_ref -n : cur prev

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "--url --token --start --verbose --debug -h -help --help" -- "$cur") )
    return
  fi

  local agents
  agents=$(_gestalt_send_cached_agents)
  COMPREPLY=( $(compgen -W "$agents" -- "$cur") )
}

complete -F _gestalt_send_complete gestalt-send
`

const zshCompletionScript = `#compdef gestalt-send
_gestalt_send_cached_agents() {
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/gestalt-send"
  local cache_file="$cache_dir/agents"
  local now
  now=$(date +%s)
  local cached=""

  if [[ -f "$cache_file" ]]; then
    local ts
    ts=$(head -n 1 "$cache_file" 2>/dev/null)
    if [[ "$ts" == <-> ]]; then
      local age=$((now - ts))
      if (( age < 60 )); then
        cached=$(tail -n +2 "$cache_file" 2>/dev/null)
      fi
    fi
  fi

  if [[ -z "$cached" ]]; then
    cached=$(gestalt-send __complete-agents 2>/dev/null)
    if [[ -n "$cached" ]]; then
      mkdir -p "$cache_dir" 2>/dev/null
      { echo "$now"; echo "$cached"; } > "$cache_file" 2>/dev/null
    fi
  fi

  echo "$cached"
}

_gestalt_send() {
  local -a flags
  flags=(
    '--url[Gestalt server URL]:url:_url'
    '--token[Gestalt auth token]:token: '
    '--start[Start agent if not running]'
    '--verbose[Verbose output]'
    '--debug[Debug output]'
  )

  _arguments -s $flags '*:agent:->agents'
  case $state in
    agents)
      local agents
      agents=(${(s: :)$(_gestalt_send_cached_agents)})
      _values 'agents' $agents
      ;;
  esac
}

_gestalt_send "$@"
`
