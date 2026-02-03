package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gestalt/internal/cli"
)

const defaultServerURL = "http://localhost:57417"
const defaultNotifyTimeout = 2 * time.Second

type Config struct {
	URL         string
	Token       string
	SessionID   string
	Payload     json.RawMessage
	Raw         string
	OccurredAt  *time.Time
	Timeout     time.Duration
	Verbose     bool
	Debug       bool
	ShowVersion bool
	LogWriter   io.Writer
}

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-notify", flag.ContinueOnError)
	fs.SetOutput(errOut)
	urlFlag := fs.String("url", "", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:57417)")
	tokenFlag := fs.String("token", "", "Auth token (env: GESTALT_TOKEN, default: none)")
	sessionIDFlag := fs.String("session-id", "", "Session ID (required)")
	timeoutFlag := fs.Duration("timeout", defaultNotifyTimeout, "Request timeout")
	verboseFlag := fs.Bool("verbose", false, "Verbose output")
	debugFlag := fs.Bool("debug", false, "Debug output (implies --verbose)")
	helpVersion := cli.AddHelpVersionFlags(fs, "Show this help message", "Print version and exit")
	fs.Usage = func() {
		printNotifyHelp(fs.Output())
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if helpVersion.Help {
		fs.Usage()
		return Config{}, flag.ErrHelp
	}

	if helpVersion.Version {
		return Config{ShowVersion: true}, nil
	}

	if fs.NArg() == 0 {
		fs.Usage()
		return Config{}, fmt.Errorf("payload is required")
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return Config{}, fmt.Errorf("invalid arguments")
	}

	sessionID := strings.TrimSpace(*sessionIDFlag)
	if sessionID == "" {
		fs.Usage()
		return Config{}, fmt.Errorf("session id required")
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

	payloadInput := strings.TrimSpace(fs.Arg(0))
	if payloadInput == "" {
		return Config{}, notifyErr(exitCodeInvalidPayload, "payload is required")
	}
	if payloadInput == "-" {
		contents, err := readPayloadFromStdin()
		if err != nil {
			return Config{}, notifyErr(exitCodeInvalidPayload, err.Error())
		}
		payloadInput = contents
	}

	payloadRaw, payloadMap, err := decodePayload(payloadInput)
	if err != nil {
		return Config{}, notifyErr(exitCodeInvalidPayload, err.Error())
	}

	occurredAt := (*time.Time)(nil)
	if payloadMap != nil {
		occurredAt = extractOccurredAt(payloadMap)
	}

	return Config{
		URL:        url,
		Token:      token,
		SessionID:  sessionID,
		Payload:    payloadRaw,
		Raw:        "",
		OccurredAt: occurredAt,
		Timeout:    *timeoutFlag,
		Verbose:    *verboseFlag,
		Debug:      *debugFlag,
	}, nil
}

func decodePayload(raw string) (json.RawMessage, map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("payload is required")
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, nil, fmt.Errorf("payload must be valid JSON")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, nil, fmt.Errorf("payload must be a JSON object")
	}
	typeValue, ok := payload["type"]
	if !ok {
		return nil, nil, fmt.Errorf("payload type is required")
	}
	typeText, ok := typeValue.(string)
	if !ok || strings.TrimSpace(typeText) == "" {
		return nil, nil, fmt.Errorf("payload type is required")
	}
	return json.RawMessage(trimmed), payload, nil
}

func extractOccurredAt(payload map[string]any) *time.Time {
	for _, key := range []string{"occurred_at", "timestamp"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
		if err != nil {
			continue
		}
		return &parsed
	}
	return nil
}

func printNotifyHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt-notify [options] <payload|->")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Send notify events to a running Gestalt session workflow")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeNotifyOption(out, "--url URL", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:57417)")
	writeNotifyOption(out, "--token TOKEN", "Auth token (env: GESTALT_TOKEN, default: none)")
	writeNotifyOption(out, "--session-id ID", "Session ID (required)")
	writeNotifyOption(out, "--timeout DURATION", "Request timeout (default: 2s)")
	writeNotifyOption(out, "--verbose", "Verbose output")
	writeNotifyOption(out, "--debug", "Debug output (implies --verbose)")
	writeNotifyOption(out, "--help", "Show this help message")
	writeNotifyOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Modes:")
	fmt.Fprintln(out, "  Manual: pass JSON payload as the final argument (use '-' for stdin)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  gestalt-notify --session-id 'Coder 1' '{\"type\":\"agent-turn-complete\"}'")
	fmt.Fprintln(out, "  echo '{\"type\":\"plan-L1-wip\",\"plan_file\":\"plan.org\"}' | gestalt-notify --session-id 'Coder 1' -")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Exit codes:")
	fmt.Fprintln(out, "  0  Success")
	fmt.Fprintln(out, "  1  Usage error")
	fmt.Fprintln(out, "  2  Request rejected")
	fmt.Fprintln(out, "  3  Network or server error")
	fmt.Fprintln(out, "  4  Session not found")
	fmt.Fprintln(out, "  5  Invalid payload")
}

func writeNotifyOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-18s %s\n", name, desc)
}

func readPayloadFromStdin() (string, error) {
	contents, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read payload from stdin: %w", err)
	}
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "" {
		return "", fmt.Errorf("payload is required")
	}
	return trimmed, nil
}
