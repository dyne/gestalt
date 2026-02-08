package shellgen

import (
	"fmt"
	"strings"
)

func BuildOllamaCommand(config map[string]interface{}) []string {
	var envs []string

	if val, ok := config["host"]; ok {
		if host, ok := val.(string); ok && strings.TrimSpace(host) != "" {
			envs = append(envs, fmt.Sprintf("OLLAMA_HOST=%s", EscapeShellArg(host)))
		}
	}

	cmd := []string{"ollama", "run"}

	if val, ok := config["model"]; ok {
		if model, ok := val.(string); ok && strings.TrimSpace(model) != "" {
			cmd = append(cmd, EscapeShellArg(model))
		}
	}

	entries := FlattenConfig(config)
	for _, entry := range entries {
		key := entry.Key
		if key == "host" || key == "model" {
			continue
		}

		flag := "--" + NormalizeFlag(key)
		switch v := entry.Value.(type) {
		case bool:
			if v {
				cmd = append(cmd, flag)
			}
		default:
			strVal := FormatValue(v)
			if strVal != "" {
				cmd = append(cmd, flag, EscapeShellArg(strVal))
			}
		}
	}

	if len(envs) > 0 {
		fullCmd := []string{"env"}
		fullCmd = append(fullCmd, envs...)
		fullCmd = append(fullCmd, cmd...)
		return fullCmd
	}

	return cmd
}
