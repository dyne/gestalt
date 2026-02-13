package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

type execRunner func(args []string) (int, error)

func runTmux(args []string) (int, error) {
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}

func formatCommand(command string, args []string) string {
	parts := []string{quoteForDisplay(command)}
	for _, arg := range args {
		parts = append(parts, quoteForDisplay(arg))
	}
	return strings.Join(parts, " ")
}

func quoteForDisplay(value string) string {
	if value == "" {
		return `""`
	}
	if !needsDisplayQuoting(value) {
		return value
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func needsDisplayQuoting(value string) bool {
	for _, r := range value {
		switch r {
		case ' ', '\t', '\n', '\r', '"', '\\':
			return true
		}
	}
	return false
}
