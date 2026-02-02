package main

import (
	"fmt"
	"io"
	"path/filepath"
)

func runAgent(cfg Config, in io.Reader, out io.Writer, exec execRunner) (int, error) {
	if err := ensureExtractedConfig(defaultConfigDir, in, out); err != nil {
		return exitConfig, err
	}
	configFS, root := buildConfigOverlay(defaultConfigDir)
	agents, err := loadAgents(configFS, root)
	if err != nil {
		return exitConfig, fmt.Errorf("load agents from %s or %s: %w", localAgentsDir(), fallbackAgentsDir(), err)
	}
	agent, err := selectAgent(agents, cfg.AgentID)
	if err != nil {
		return exitAgent, fmt.Errorf("%w (expected agent file at %s or %s)", err, localAgentPath(cfg.AgentID), fallbackAgentPath(cfg.AgentID))
	}
	resolver := defaultPortResolver()
	developerPrompt, err := renderDeveloperPrompt(agent, configFS, root, resolver)
	if err != nil {
		return exitPrompt, fmt.Errorf("%w (prompt roots: %s, %s)", err, localPromptsDir(), fallbackPromptsDir())
	}
	if developerPrompt == "" {
		// Ensure deterministic behavior even when no prompts are configured.
		developerPrompt = ""
	}
	args := buildCodexArgs(agent.CLIConfig, developerPrompt)
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
