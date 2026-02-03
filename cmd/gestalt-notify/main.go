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
	return runWithSender(args, out, errOut, sendNotifyEvent)
}

func runWithSender(args []string, out io.Writer, errOut io.Writer, send func(Config) error) int {
	cfg, err := parseArgs(args, errOut)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitCodeSuccess
		}
		var notifyErr *notifyError
		if errors.As(err, &notifyErr) {
			fmt.Fprintln(errOut, notifyErr.Message)
			return notifyErr.Code
		}
		return exitCodeUsage
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(out, "gestalt-notify dev")
		} else {
			fmt.Fprintf(out, "gestalt-notify version %s\n", version.Version)
		}
		return exitCodeSuccess
	}
	if cfg.Debug {
		cfg.Verbose = true
	}
	cfg.LogWriter = errOut
	applyTimeout(cfg)

	if send == nil {
		return exitCodeSuccess
	}
	if err := send(cfg); err != nil {
		return handleNotifyError(err, errOut)
	}
	return exitCodeSuccess
}
