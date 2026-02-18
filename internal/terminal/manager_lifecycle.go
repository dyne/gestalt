package terminal

import (
	"strings"

	"gestalt/internal/event"
)

func (m *Manager) emitSessionStarted(id string, request sessionCreateRequest, agentName, shell string) {
	fields := map[string]string{
		"gestalt.category": "terminal",
		"gestalt.source":   "backend",
		"session.id":       id,
		"role":             request.Role,
		"title":            request.Title,
	}
	if request.AgentID != "" {
		fields["agent.id"] = request.AgentID
		fields["agent_id"] = request.AgentID
		if strings.TrimSpace(agentName) != "" {
			fields["agent.name"] = agentName
			fields["agent_name"] = agentName
		}
		if strings.TrimSpace(shell) != "" {
			fields["shell"] = redactDeveloperInstructionsShell(shell)
		}
	}
	m.logger.Info("session created", fields)
	if m.terminalBus != nil {
		m.terminalBus.Publish(event.NewTerminalEvent(id, "terminal_created"))
	}
	if request.AgentID != "" && m.agentBus != nil {
		m.agentBus.Publish(event.NewAgentEvent(request.AgentID, agentName, "agent_started"))
	}
}

func (m *Manager) emitSessionStopped(id string, session *Session, agentID, agentName string, closeErr error) {
	if closeErr != nil {
		fields := map[string]string{
			"gestalt.category": "terminal",
			"gestalt.source":   "backend",
			"session.id":       id,
			"error":            closeErr.Error(),
		}
		if agentID != "" {
			fields["agent.id"] = agentID
			fields["agent_id"] = agentID
		}
		if strings.TrimSpace(agentName) != "" {
			fields["agent.name"] = agentName
			fields["agent_name"] = agentName
		}
		if session != nil {
			if tail := renderOutputTail(m.logger, session.OutputLines(), 12, 2000); tail != "" {
				fields["output_tail"] = tail
			}
		}
		m.logger.Warn("session close error", fields)
		if m.terminalBus != nil {
			terminalEvent := event.NewTerminalEvent(id, "terminal_error")
			terminalEvent.Data = map[string]any{
				"error": closeErr.Error(),
			}
			m.terminalBus.Publish(terminalEvent)
		}
		if agentID != "" && m.agentBus != nil {
			agentEvent := event.NewAgentEvent(agentID, agentName, "agent_error")
			agentEvent.Context = map[string]any{
				"error": closeErr.Error(),
			}
			m.agentBus.Publish(agentEvent)
		}
	}
	if m.terminalBus != nil {
		m.terminalBus.Publish(event.NewTerminalEvent(id, "terminal_closed"))
	}
	if agentID != "" && m.agentBus != nil {
		m.agentBus.Publish(event.NewAgentEvent(agentID, agentName, "agent_stopped"))
	}
	m.logger.Info("session deleted", map[string]string{
		"gestalt.category": "terminal",
		"gestalt.source":   "backend",
		"session.id":       id,
	})
}
