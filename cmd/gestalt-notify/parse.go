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
	EventType   string
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
	eventTypeFlag := fs.String("event-type", "", "Event type (required when payload lacks type)")
	payloadFlag := fs.String("payload", "", "JSON payload string (manual mode)")
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

	if fs.NArg() > 1 {
		fs.Usage()
		return Config{}, fmt.Errorf("invalid arguments")
	}

	rawArg := ""
	if fs.NArg() == 1 {
		rawArg = strings.TrimSpace(fs.Arg(0))
		if rawArg == "" {
			fs.Usage()
			return Config{}, fmt.Errorf("payload JSON is required")
		}
	}

	payloadInput := strings.TrimSpace(*payloadFlag)
	if rawArg != "" && payloadInput != "" {
		fs.Usage()
		return Config{}, fmt.Errorf("payload provided twice")
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

	payloadRaw, payloadMap, err := decodePayload(rawArg)
	if err != nil {
		fs.Usage()
		return Config{}, err
	}
	if payloadRaw == nil && payloadInput != "" {
		payloadRaw, payloadMap, err = decodePayload(payloadInput)
		if err != nil {
			fs.Usage()
			return Config{}, err
		}
	}

	eventType := strings.TrimSpace(*eventTypeFlag)
	if eventType == "" && payloadMap != nil {
		eventType = extractEventType(payloadMap)
	}
	if eventType == "" {
		fs.Usage()
		return Config{}, fmt.Errorf("event type required")
	}

	occurredAt := (*time.Time)(nil)
	if payloadMap != nil {
		occurredAt = extractOccurredAt(payloadMap)
	}

	return Config{
		URL:        url,
		Token:      token,
		SessionID:  sessionID,
		EventType:  eventType,
		Payload:    payloadRaw,
		Raw:        rawArg,
		OccurredAt: occurredAt,
		Timeout:    *timeoutFlag,
		Verbose:    *verboseFlag,
		Debug:      *debugFlag,
	}, nil
}

func decodePayload(raw string) (json.RawMessage, map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, nil, fmt.Errorf("payload must be valid JSON")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, nil, fmt.Errorf("payload must be a JSON object")
	}
	return json.RawMessage(trimmed), payload, nil
}

func extractEventType(payload map[string]any) string {
	for _, key := range []string{"type", "event_type"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	fmt.Fprintln(out, "Usage: gestalt-notify [options] [json-payload]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Send notify events to a running Gestalt session workflow")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeNotifyOption(out, "--url URL", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:57417)")
	writeNotifyOption(out, "--token TOKEN", "Auth token (env: GESTALT_TOKEN, default: none)")
	writeNotifyOption(out, "--session-id ID", "Session ID (required)")
	writeNotifyOption(out, "--event-type TYPE", "Event type (required when payload lacks type)")
	writeNotifyOption(out, "--payload JSON", "JSON payload string (manual mode)")
	writeNotifyOption(out, "--timeout DURATION", "Request timeout (default: 2s)")
	writeNotifyOption(out, "--verbose", "Verbose output")
	writeNotifyOption(out, "--debug", "Debug output (implies --verbose)")
	writeNotifyOption(out, "--help", "Show this help message")
	writeNotifyOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Modes:")
	fmt.Fprintln(out, "  Codex notifier: supply JSON payload as final arg")
	fmt.Fprintln(out, "  Manual: use --event-type and optional --payload")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  gestalt-notify --session-id 'Coder 1' '{\"type\":\"agent-turn-complete\"}'")
	fmt.Fprintln(out, "  gestalt-notify --session-id 'Coder 1' --event-type plan-L1-wip --payload '{\"plan_file\":\"plan.org\"}'")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Exit codes:")
	fmt.Fprintln(out, "  0  Success")
	fmt.Fprintln(out, "  1  Usage error")
	fmt.Fprintln(out, "  2  Request rejected")
	fmt.Fprintln(out, "  3  Network or server error")
}

func writeNotifyOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-18s %s\n", name, desc)
}
