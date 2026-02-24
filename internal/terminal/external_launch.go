package terminal

import (
	"errors"
	"gestalt/internal/runner/launchspec"
	"strings"
)

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

func buildPromptInjectionSpec(payloads []string) launchspec.PromptInjectionSpec {
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

func (m *Manager) buildLaunchSpec(session *Session, promptPayloads []string) *launchspec.LaunchSpec {
	if session == nil {
		return nil
	}
	var argv []string
	if strings.TrimSpace(session.Command) != "" {
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
		PromptInjection: buildPromptInjectionSpec(promptPayloads),
	}
	normalized := launchspec.NormalizeLaunchSpec(spec)
	return &normalized
}
