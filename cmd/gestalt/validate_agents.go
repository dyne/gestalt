package main

import (
	"bytes"
	"errors"
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
	warnLegacyMemoBlobs(agentsDir, errOut)
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

const legacyMemoScanLimit = 2 * 1024 * 1024

var legacyMemoPatterns = [][]byte{
	[]byte(`"agent_config":{`),
	[]byte(`"agent_config":[`),
	[]byte(`"agent_config":"{`),
	[]byte(`"agent_config":"[`),
}

func warnLegacyMemoBlobs(agentsDir string, errOut io.Writer) {
	for _, candidate := range legacyMemoCandidates(agentsDir) {
		found, err := containsLegacyMemoInDir(candidate)
		if err != nil || !found {
			continue
		}
		fmt.Fprintf(errOut, "WARNING: legacy JSON agent_config memos detected in %s; restart workflows to regenerate TOML memos\n", candidate)
		return
	}
}

func legacyMemoCandidates(agentsDir string) []string {
	candidates := []string{filepath.Join(".gestalt", "temporal")}
	if root := gestaltRootFromAgentsDir(agentsDir); root != "" {
		candidates = append(candidates, filepath.Join(root, "temporal"))
	}
	seen := make(map[string]struct{})
	var unique []string
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		unique = append(unique, cleaned)
	}
	return unique
}

func gestaltRootFromAgentsDir(agentsDir string) string {
	absPath, err := filepath.Abs(agentsDir)
	if err != nil {
		return ""
	}
	current := absPath
	for {
		if filepath.Base(current) == ".gestalt" {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func containsLegacyMemoInDir(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	var found bool
	errLegacyMemo := errors.New("legacy memo found")
	walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found {
			return errLegacyMemo
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		matches, err := containsLegacyMemoInFile(path, info.Size())
		if err != nil {
			return err
		}
		if matches {
			found = true
			return errLegacyMemo
		}
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, errLegacyMemo) {
			return true, nil
		}
		return false, walkErr
	}
	return found, nil
}

func containsLegacyMemoInFile(path string, size int64) (bool, error) {
	if size <= 0 {
		return false, nil
	}
	limit := legacyMemoScanLimit
	if size <= int64(limit) {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		return containsLegacyMemoData(data), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	buffer := make([]byte, limit)
	readBytes, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return containsLegacyMemoData(buffer[:readBytes]), nil
}

func containsLegacyMemoData(data []byte) bool {
	for _, pattern := range legacyMemoPatterns {
		if len(data) >= len(pattern) && bytes.Contains(data, pattern) {
			return true
		}
	}
	return false
}
