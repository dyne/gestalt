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
	return runWithSender(args, in, errOut, sendInput)
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

	if cfg.SessionID == "" {
		if err := resolveAgent(&cfg); err != nil {
			return handleSendError(err, errOut)
		}
	}
	if err := ensureSession(&cfg); err != nil {
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
