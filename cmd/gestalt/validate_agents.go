package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/agent"
)

func runValidateConfig(args []string) int {
	return runValidateConfigWithOutput(args, os.Stdout, os.Stderr)
}

func runValidateConfigWithOutput(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("gestalt config validate", flag.ContinueOnError)
	fs.SetOutput(errOut)
	agentsDir := fs.String("agents-dir", filepath.Join(".gestalt", "config", "agents"), "Agents directory")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	return validateAgentsDir(*agentsDir, out, errOut)
}

func validateAgentsDir(agentsDir string, out, errOut io.Writer) int {
	agentsDir = strings.TrimSpace(agentsDir)
	if agentsDir == "" {
		fmt.Fprintln(errOut, "agents-dir is required")
		return 1
	}
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		fmt.Fprintf(errOut, "read agents dir: %v\n", err)
		return 1
	}

	validCount := 0
	invalidCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".toml" {
			if ext == ".json" {
				invalidCount++
				fmt.Fprintf(out, "ERROR %s: only TOML agent configs are supported\n", name)
			}
			continue
		}
		agentID := strings.TrimSuffix(name, ext)
		profile, err := agent.LoadAgentByID(agentID, agentsDir)
		if err != nil {
			invalidCount++
			fmt.Fprintf(out, "ERROR %s: %v\n", name, err)
			continue
		}
		validCount++
		if profile != nil && strings.TrimSpace(profile.Name) != "" {
			fmt.Fprintf(out, "OK %s (%s)\n", agentID, profile.Name)
		} else {
			fmt.Fprintf(out, "OK %s\n", agentID)
		}
	}

	fmt.Fprintf(out, "Summary: %d valid, %d invalid\n", validCount, invalidCount)
	if invalidCount > 0 {
		return 1
	}
	return 0
}
