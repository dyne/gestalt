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
		entries := shellgen.FlattenConfig(config)
		args := []string{"copilot"}
		for _, entry := range entries {
			flag := "--" + shellgen.NormalizeFlag(entry.Key)
			switch value := entry.Value.(type) {
			case bool:
				if value {
					args = append(args, flag)
				} else {
					args = append(args, "--no-"+shellgen.NormalizeFlag(entry.Key))
				}
			default:
				args = append(args, flag, shellgen.EscapeShellArg(shellgen.FormatValue(value)))
			}
		}
		return args
	default:
		return nil
	}
}
