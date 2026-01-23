package agent

import (
	"strings"

	"gestalt/internal/agent/shellgen"
)

// BuildShellCommand builds a shell command string from cli_config settings.
func BuildShellCommand(cliType string, config map[string]interface{}) string {
	args := buildShellArgs(cliType, config)
	if len(args) == 0 {
		return ""
	}
	return strings.Join(args, " ")
}

func buildShellArgs(cliType string, config map[string]interface{}) []string {
	cliType = strings.ToLower(strings.TrimSpace(cliType))
	if cliType == "" {
		return nil
	}

	switch cliType {
	case "codex":
		return shellgen.BuildCodexCommand(config)
	case "copilot":
		return shellgen.BuildCopilotCommand(config)
	default:
		return nil
	}
}
