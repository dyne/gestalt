package agent

import (
	"fmt"

	"gestalt/internal/agent/shellgen"
)

// BuildCodexArgs builds Codex CLI arguments with developer instructions injected.
func BuildCodexArgs(config map[string]interface{}, developerPrompt string) []string {
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
