package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"gestalt/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out io.Writer, errOut io.Writer) int {
	return runWithExec(args, out, errOut, runTmux)
}

func runWithExec(args []string, out io.Writer, errOut io.Writer, exec execRunner) int {
	cfg, err := parseArgs(args, errOut)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(errOut, err)
		return exitUsage
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(out, "gestalt-agent dev")
		} else {
			fmt.Fprintf(out, "gestalt-agent version %s\n", version.Version)
		}
		return 0
	}

	runner := exec
	if runner == nil {
		runner = runTmux
	}
	if cfg.DryRun {
		runner = func(args []string) (int, error) {
			fmt.Fprintln(out, formatCommand("tmux", args))
			return 0, nil
		}
	}

	exitCode, err := runAgent(cfg, os.Stdin, out, runner)
	if err != nil {
		fmt.Fprintln(errOut, err)
	}
	if exitCode != 0 {
		return exitCode
	}
	if err != nil {
		return 1
	}
	return 0
}
