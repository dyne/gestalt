package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gestalt/internal/agent/shellgen"
)

type execRunner func(args []string) (int, error)

func buildCodexArgs(config map[string]interface{}, developerPrompt string) []string {
	args := []string{}
	for _, entry := range shellgen.FlattenConfigPreserveArrays(config) {
		if entry.Key == "" {
			continue
		}
		if entry.Key == "developer_instructions" {
			continue
		}
		if entry.Key == "notify" {
			if single, ok := entry.Value.(string); ok {
				entry.Value = []string{single}
			}
		}
		value := shellgen.FormatValue(entry.Value)
		args = append(args, "-c", fmt.Sprintf("%s=%s", entry.Key, value))
	}
	args = append(args, "-c", fmt.Sprintf("developer_instructions=%s", developerPrompt))
	return args
}

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
