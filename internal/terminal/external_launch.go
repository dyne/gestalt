package terminal

import (
	"errors"
	"strings"

	"gestalt/internal/agent"
	"gestalt/internal/guimodules"
	"gestalt/internal/runner/launchspec"
)

var defaultExternalGUIModules = []string{guimodules.ModuleConsole, guimodules.ModulePlanProgress}
var defaultServerGUIModules = []string{guimodules.ModuleConsole}

func normalizeRunnerKind(value string) (launchspec.RunnerKind, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" || trimmed == string(launchspec.RunnerKindServer) {
		return launchspec.RunnerKindServer, nil
	}
	if trimmed == string(launchspec.RunnerKindExternal) {
		return launchspec.RunnerKindExternal, nil
	}
	return "", errors.New("invalid runner kind")
}

func normalizeSessionGUIModules(modules []string) []string {
	return guimodules.Normalize(modules)
}

func (m *Manager) buildExternalPromptPayloads(promptNames []string, sessionID string) ([]string, []string) {
	if len(promptNames) == 0 {
		return nil, nil
	}
	payloads := make([]string, 0, len(promptNames))
	files := make([]string, 0, len(promptNames))
	for _, promptName := range promptNames {
		trimmed := strings.TrimSpace(promptName)
		if trimmed == "" {
			continue
		}
		data, promptFiles, err := m.readPromptFile(trimmed, sessionID)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("agent prompt file read failed", map[string]string{
					"agent_id":   "",
					"prompt":     trimmed,
					"session_id": sessionID,
					"error":      err.Error(),
				})
			}
			continue
		}
		if len(data) == 0 {
			continue
		}
		payloads = append(payloads, string(data))
		files = append(files, promptFiles...)
	}
	if len(payloads) == 0 {
		return nil, nil
	}
	return payloads, files
}

func buildPromptInjectionSpec(cliType string, payloads []string) launchspec.PromptInjectionSpec {
	if strings.EqualFold(strings.TrimSpace(cliType), "codex") {
		return launchspec.NormalizePromptInjection(launchspec.PromptInjectionSpec{
			Mode: launchspec.PromptInjectionCodexDeveloperInstructions,
		})
	}
	if len(payloads) == 0 {
		return launchspec.NormalizePromptInjection(launchspec.PromptInjectionSpec{
			Mode: launchspec.PromptInjectionNone,
		})
	}
	return launchspec.NormalizePromptInjection(launchspec.PromptInjectionSpec{
		Mode:    launchspec.PromptInjectionStdin,
		Payload: payloads,
		Pacing:  launchspec.DefaultPromptInjectionPacing(),
	})
}

func (m *Manager) buildLaunchSpec(session *Session, profile *agent.Agent, cliConfig map[string]interface{}, developerInstructions string, promptPayloads []string) *launchspec.LaunchSpec {
	if session == nil {
		return nil
	}
	cliType := ""
	if profile != nil {
		cliType = profile.CLIType
	}
	argv := launchspec.BuildArgv(cliType, cliConfig, developerInstructions)
	if len(argv) == 0 && strings.TrimSpace(session.Command) != "" {
		command, args, err := splitCommandLine(session.Command)
		if err == nil {
			argv = append([]string{command}, args...)
		}
	}
	info := session.Info()
	spec := launchspec.LaunchSpec{
		SessionID:       info.ID,
		Argv:            argv,
		Interface:       info.Interface,
		PromptFiles:     info.PromptFiles,
		GUIModules:      info.GUIModules,
		PromptInjection: buildPromptInjectionSpec(cliType, promptPayloads),
	}
	normalized := launchspec.NormalizeLaunchSpec(spec)
	return &normalized
}
