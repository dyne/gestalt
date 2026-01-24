package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gestalt/internal/cli"
)

const defaultServerURL = "http://localhost:57417"

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

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-send", flag.ContinueOnError)
	fs.SetOutput(errOut)
	urlFlag := fs.String("url", "", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:57417)")
	tokenFlag := fs.String("token", "", "Auth token (env: GESTALT_TOKEN, default: none)")
	startFlag := fs.Bool("start", false, "Start agent if not running")
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
	writeSendOption(out, "--url URL", "Gestalt server URL (env: GESTALT_URL, default: http://localhost:57417)")
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
	fmt.Fprintln(out, "  gestalt-send --url http://remote:57417 --token abc123 agent-id")
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
