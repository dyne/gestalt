package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gestalt/internal/cli"
	"gestalt/internal/client"
)

const defaultServerHost = "127.0.0.1"
const defaultServerPort = 57417

type Config struct {
	URL         string
	Token       string
	SessionID   string
	Verbose     bool
	Debug       bool
	ShowVersion bool
	LogWriter   io.Writer
}

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-send", flag.ContinueOnError)
	fs.SetOutput(errOut)
	hostFlag := fs.String("host", defaultServerHost, "Gestalt server host")
	portFlag := fs.Int("port", defaultServerPort, "Gestalt server port")
	sessionIDFlag := fs.String("session-id", "", "Target session id")
	tokenFlag := fs.String("token", "", "Auth token (env: GESTALT_TOKEN, default: none)")
	verboseFlag := fs.Bool("verbose", false, "Verbose output")
	debugFlag := fs.Bool("debug", false, "Debug output (implies --verbose)")
	helpVersion := cli.AddHelpVersionFlags(fs, "Show this help message", "Print version and exit")
	fs.Usage = func() {
		printSendHelp(fs.Output())
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

	if fs.NArg() > 0 {
		fs.Usage()
		return Config{}, fmt.Errorf("unexpected positional arguments")
	}

	sessionID := strings.TrimSpace(*sessionIDFlag)
	normalizedSessionID, err := client.NormalizeSessionRef(sessionID)
	if err != nil {
		fs.Usage()
		return Config{}, err
	}
	sessionID = normalizedSessionID
	if *portFlag <= 0 || *portFlag > 65535 {
		fs.Usage()
		return Config{}, fmt.Errorf("port must be between 1 and 65535")
	}

	host := strings.TrimSpace(*hostFlag)
	if host == "" {
		host = defaultServerHost
	}
	baseURL := buildServerURL(host, *portFlag)

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
	}

	return Config{
		URL:       baseURL,
		Token:     token,
		SessionID: sessionID,
		Verbose:   *verboseFlag,
		Debug:     *debugFlag,
	}, nil
}

func printSendHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt-send [options] --session-id <id>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Send stdin to a running Gestalt session")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeSendOption(out, "--host HOST", "Gestalt server host (default: 127.0.0.1)")
	writeSendOption(out, "--port PORT", "Gestalt server port (default: 57417)")
	writeSendOption(out, "--session-id ID", "Target session id (required)")
	writeSendOption(out, "--token TOKEN", "Auth token (env: GESTALT_TOKEN, default: none)")
	writeSendOption(out, "--verbose", "Show request/response details")
	writeSendOption(out, "--debug", "Show detailed debug info (implies --verbose)")
	writeSendOption(out, "--help", "Show this help message")
	writeSendOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  echo \"status\" | gestalt-send --session-id session-1")
	fmt.Fprintln(out, "  cat file.txt | gestalt-send --host remote --port 57417 --token abc123 --session-id session-1")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Exit codes:")
	fmt.Fprintln(out, "  0  Success")
	fmt.Fprintln(out, "  1  Usage error")
	fmt.Fprintln(out, "  2  Session not found")
	fmt.Fprintln(out, "  3  Network or server error")
}

func buildServerURL(host string, port int) string {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		trimmedHost = defaultServerHost
	}
	trimmedHost = strings.TrimPrefix(trimmedHost, "http://")
	trimmedHost = strings.TrimPrefix(trimmedHost, "https://")
	return "http://" + trimmedHost + ":" + strconv.Itoa(port)
}

func writeSendOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-14s %s\n", name, desc)
}
