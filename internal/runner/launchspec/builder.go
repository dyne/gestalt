package launchspec

import (
	"strings"

	"gestalt/internal/agent"
	"gestalt/internal/agent/shellgen"
)

// BuildArgv builds the argv list for a CLI type and config.
func BuildArgv(cliType string, config map[string]interface{}, developerPrompt string) []string {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	switch cliType {
	case "codex":
		args := agent.BuildCodexArgs(config, developerPrompt)
		if len(args) == 0 {
			return []string{"codex"}
		}
		return append([]string{"codex"}, args...)
	case "copilot":
		return shellgen.BuildCopilotCommand(config)
	default:
		return nil
	}
}
