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
}

func parseArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-agent", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dryRun := fs.Bool("dryrun", false, "Print the codex command without executing")
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
	fmt.Fprintln(out, "Run Codex using a Gestalt agent configuration")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	writeOption(out, "--dryrun", "Print the command without executing")
	writeOption(out, "--help", "Show this help message")
	writeOption(out, "--version", "Print version and exit")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Arguments:")
	fmt.Fprintln(out, "  agent-id-or-filename  Agent filename in config/agents (ex: coder or coder.toml)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  gestalt-agent coder")
	fmt.Fprintln(out, "  gestalt-agent coder.toml")
}

func writeOption(out io.Writer, name, desc string) {
	fmt.Fprintf(out, "  %-12s %s\n", name, desc)
}
