package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type execRunner func(args []string) (int, error)

func runCodex(args []string) (int, error) {
	cmd := exec.Command("codex", args...)
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

func formatCodexCommand(args []string) string {
	parts := []string{"codex"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-c" && i+1 < len(args) {
			entry := args[i+1]
			i++
			key, value, ok := strings.Cut(entry, "=")
			if ok && key == "developer_instructions" {
				entry = fmt.Sprintf(`%s="%s"`, key, escapeDeveloperPrompt(value))
				parts = append(parts, "-c", entry)
				continue
			}
			parts = append(parts, "-c", quoteForDisplay(entry))
			continue
		}
		parts = append(parts, quoteForDisplay(arg))
	}
	return strings.Join(parts, " ")
}

func escapeDeveloperPrompt(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
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
