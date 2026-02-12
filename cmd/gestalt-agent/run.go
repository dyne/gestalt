package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	agentpkg "gestalt/internal/agent"
)

func runAgent(cfg Config, in io.Reader, out io.Writer, exec execRunner) (int, error) {
	if err := ensureExtractedConfig(defaultConfigDir, in, out); err != nil {
		return exitConfig, err
	}
	configFS, root := buildConfigOverlay(defaultConfigDir)
	agents, skills, err := loadAgentsAndSkills(configFS, root)
	if err != nil {
		return exitConfig, fmt.Errorf("load agents from %s or %s: %w", localAgentsDir(), fallbackAgentsDir(), err)
	}
	profile, err := selectAgent(agents, cfg.AgentID)
	if err != nil {
		return exitAgent, fmt.Errorf("%w (expected agent file at %s or %s)", err, localAgentPath(cfg.AgentID), fallbackAgentPath(cfg.AgentID))
	}
	if strings.EqualFold(profile.Interface, agentpkg.AgentInterfaceMCP) {
		fmt.Fprintln(os.Stderr, `interface="mcp" ignored by gestalt-agent; affects server/UI only`)
	}
	resolver := defaultPortResolver()
	developerPrompt, err := renderDeveloperPrompt(profile, skills, configFS, root, resolver)
	if err != nil {
		return exitPrompt, fmt.Errorf("%w (prompt roots: %s, %s)", err, localPromptsDir(), fallbackPromptsDir())
	}
	if developerPrompt == "" {
		// Ensure deterministic behavior even when no prompts are configured.
		developerPrompt = ""
	}
	if profile.CLIConfig != nil {
		if _, ok := profile.CLIConfig["developer_instructions"]; ok {
			fmt.Fprintln(os.Stderr, "warning: developer_instructions overridden by rendered prompt")
		}
	}
	args := agentpkg.BuildCodexArgs(profile.CLIConfig, developerPrompt)
	if exec == nil {
		return 0, nil
	}
	return exec(args)
}

func localAgentsDir() string {
	return filepath.Join("config", "agents")
}

func fallbackAgentsDir() string {
	return filepath.Join(defaultConfigDir, "agents")
}

func localAgentPath(agentID string) string {
	return filepath.Join(localAgentsDir(), agentID+".toml")
}

func fallbackAgentPath(agentID string) string {
	return filepath.Join(fallbackAgentsDir(), agentID+".toml")
}

func localPromptsDir() string {
	return filepath.Join("config", "prompts")
}

func fallbackPromptsDir() string {
	return filepath.Join(defaultConfigDir, "prompts")
}
