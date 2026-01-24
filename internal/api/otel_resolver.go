package api

import (
	"net/http"
	"strings"

	"gestalt/internal/otel"
	"gestalt/internal/terminal"
)

func apiAgentResolver(manager *terminal.Manager) otel.AgentResolver {
	if manager == nil {
		return nil
	}
	return func(r *http.Request, bodyAgentID string) otel.AgentAttributes {
		info := otel.AgentAttributes{Type: "unknown"}
		if r == nil {
			return info
		}

		if bodyAgentID != "" {
			info.ID = bodyAgentID
			if profile, ok := manager.GetAgent(bodyAgentID); ok {
				info.Name = profile.Name
				info.Type = normalizeAgentType(profile.CLIType)
			}
			return info
		}

		if agentName := agentNameFromPath(r.URL.Path); agentName != "" {
			info.Name = agentName
			if session, ok := manager.GetSessionByAgent(agentName); ok && session != nil {
				return agentFromSession(manager, session)
			}
			if agentID, agentType := lookupAgentByName(manager.ListAgents(), agentName); agentID != "" {
				info.ID = agentID
				info.Type = normalizeAgentType(agentType)
				return info
			}
			return info
		}

		if terminalID := terminalIDFromPath(r.URL.Path); terminalID != "" {
			info.TerminalID = terminalID
			if session, ok := manager.Get(terminalID); ok && session != nil {
				return agentFromSession(manager, session)
			}
		}

		return info
	}
}

func agentFromSession(manager *terminal.Manager, session *terminal.Session) otel.AgentAttributes {
	info := otel.AgentAttributes{Type: "unknown"}
	if session == nil {
		return info
	}
	info.TerminalID = session.ID
	info.ID = session.AgentID
	info.Type = normalizeAgentType(session.LLMType)
	if info.ID != "" {
		if profile, ok := manager.GetAgent(info.ID); ok {
			info.Name = profile.Name
			if profile.CLIType != "" {
				info.Type = normalizeAgentType(profile.CLIType)
			}
		}
	}
	return info
}

func normalizeAgentType(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func lookupAgentByName(agents []terminal.AgentInfo, name string) (string, string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ""
	}
	for _, info := range agents {
		if info.Name == trimmed {
			return info.ID, info.LLMType
		}
	}
	return "", ""
}

func terminalIDFromPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return ""
	}
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func agentNameFromPath(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	const prefix = "/api/agents/"
	if !strings.HasPrefix(trimmed, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "input" {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
