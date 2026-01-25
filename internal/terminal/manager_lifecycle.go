package terminal

import (
	"strings"

	"gestalt/internal/event"
)

func (m *Manager) emitSessionStarted(id string, request sessionCreateRequest, agentName, shell string) {
	fields := map[string]string{
		"terminal_id": id,
		"role":        request.Role,
		"title":       request.Title,
	}
	if request.AgentID != "" {
		fields["agent_id"] = request.AgentID
		if strings.TrimSpace(shell) != "" {
			fields["shell"] = shell
		}
	}
	m.logger.Info("terminal created", fields)
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
			"terminal_id": id,
			"error":       closeErr.Error(),
		}
		if session != nil {
			if tail := renderOutputTail(m.logger, session.OutputLines(), 12, 2000); tail != "" {
				fields["output_tail"] = tail
			}
		}
		m.logger.Warn("terminal close error", fields)
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
	if session != nil {
		if workflowID, workflowRunID, ok := session.WorkflowIdentifiers(); ok {
			m.logger.Info("workflow stopped", map[string]string{
				"terminal_id": id,
				"workflow_id": workflowID,
				"run_id":      workflowRunID,
			})
		}
	}
	m.logger.Info("terminal deleted", map[string]string{
		"terminal_id": id,
	})
}
