package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"gestalt/internal/cli"
)

type Config struct {
	AgentArg    string
	AgentID     string
	DryRun      bool
	ShowVersion bool
	URL         string
	Token       string
}

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-agent", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dryRun := fs.Bool("dryrun", false, "Print the codex command without executing")
	url := fs.String("url", defaultGestaltURL(), "Gestalt server URL")
	token := fs.String("token", defaultGestaltToken(), "Gestalt auth token")
	helper := cli.AddHelpVersionFlags(fs, "Show this help message", "Print version and exit")
	fs.Usage = func() {
		printHelp(fs.Output())
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if helper.Help {
		fs.Usage()
		return Config{}, flag.ErrHelp
	}

	if helper.Version {
		return Config{ShowVersion: true}, nil
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return Config{}, fmt.Errorf("agent id required")
	}

	agentArg := strings.TrimSpace(fs.Arg(0))
	if agentArg == "" {
		fs.Usage()
		return Config{}, fmt.Errorf("agent id required")
	}
	if strings.Contains(agentArg, "/") || strings.Contains(agentArg, "\\") {
		fs.Usage()
		return Config{}, fmt.Errorf("agent id must not be a path")
	}

	agentID := trimTomlSuffix(agentArg)
	if agentID == "" {
		fs.Usage()
		return Config{}, fmt.Errorf("agent id required")
	}

	return Config{
		AgentArg: agentArg,
		AgentID:  agentID,
		DryRun:   *dryRun,
		URL:      *url,
		Token:    *token,
	}, nil
}

func trimTomlSuffix(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasSuffix(strings.ToLower(trimmed), ".toml") {
		trimmed = trimmed[:len(trimmed)-len(".toml")]
	}
	return strings.TrimSpace(trimmed)
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt-agent <agent-id-or-filename>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Register an external session with a Gestalt server and run the agent in tmux")
	fmt.Fprintln(out, "Requires a running server and tmux on PATH")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeOption(out, "--dryrun", "Print the resolved command without starting tmux")
	writeOption(out, "--url", "Gestalt server URL (env: GESTALT_URL)")
	writeOption(out, "--token", "Gestalt auth token (env: GESTALT_TOKEN)")
	writeOption(out, "--help", "Show this help message")
	writeOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Arguments:")
	fmt.Fprintln(out, "  agent-id-or-filename  Agent filename in .gestalt/config/agents (ex: coder or coder.toml)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  gestalt-agent coder")
	fmt.Fprintln(out, "  gestalt-agent coder.toml")
}

func writeOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-12s %s\n", name, desc)
}
