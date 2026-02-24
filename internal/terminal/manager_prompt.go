package terminal

import (
	"strconv"
	"strings"
	"time"

	"gestalt/internal/agent"
	"gestalt/internal/skill"
)

func (m *Manager) startPromptInjection(session *Session, agentID string, profile *agent.Agent, promptNames []string, onAirString string) {
	if session == nil || profile == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(profile.RuntimeType()), "codex") {
		return
	}
	if len(profile.Skills) == 0 && len(promptNames) == 0 {
		return
	}

	go func() {
		// Wait for shell to be ready
		if strings.TrimSpace(onAirString) != "" {
			if !waitForOnAir(session, onAirString, onAirTimeout) {
				m.logger.Error("agent onair string not found", map[string]string{
					"agent_id":     agentID,
					"onair_string": onAirString,
					"timeout_ms":   strconv.FormatInt(onAirTimeout.Milliseconds(), 10),
				})
			}
		} else {
			time.Sleep(promptDelay)
		}

		// Inject skill metadata first if agent has skills
		if len(profile.Skills) > 0 {
			agentSkills := make([]*skill.Skill, 0, len(profile.Skills))
			for _, skillName := range profile.Skills {
				if skillEntry, ok := m.skills[skillName]; ok {
					agentSkills = append(agentSkills, skillEntry)
				}
			}

			if len(agentSkills) > 0 {
				// Build metadata for LLM system prompt only; do not write to terminal.
				_ = skill.GeneratePromptXML(agentSkills)
				m.logger.Info("agent skill metadata prepared", map[string]string{
					"agent_id":    agentID,
					"skill_count": strconv.Itoa(len(agentSkills)),
				})
				time.Sleep(interPromptDelay)
			}
		}

		// Inject custom prompts
		if len(promptNames) > 0 {
			wrotePrompt := false
			cleaned := make([]string, 0, len(promptNames))
			for _, promptName := range promptNames {
				promptName = strings.TrimSpace(promptName)
				if promptName != "" {
					cleaned = append(cleaned, promptName)
				}
			}
			for i, promptName := range cleaned {
				data, files, err := m.readPromptFile(promptName, session.ID)
				if err != nil {
					m.logger.Warn("agent prompt file read failed", map[string]string{
						"agent_id": agentID,
						"prompt":   promptName,
						"error":    err.Error(),
					})
					continue
				}
				if err := writePromptPayload(session, data); err != nil {
					fields := map[string]string{
						"agent_id": agentID,
						"prompt":   promptName,
						"error":    err.Error(),
					}
					if tail := renderOutputTail(m.logger, session.OutputLines(), 12, 2000); tail != "" {
						fields["output_tail"] = tail
					}
					m.logger.Error("agent prompt write failed", fields)
					return
				}
				session.PromptFiles = append(session.PromptFiles, files...)
				m.logger.Info("agent prompt rendered", map[string]string{
					"agent_id":     agentID,
					"agent_name":   profile.Name,
					"prompt_files": strings.Join(files, ", "),
					"file_count":   strconv.Itoa(len(files)),
				})
				wrotePrompt = true
				if i < len(cleaned)-1 {
					time.Sleep(interPromptDelay)
				}
			}
			if wrotePrompt {
				time.Sleep(finalEnterDelay)
				if err := session.Write([]byte("\r")); err != nil {
					m.logger.Warn("agent prompt final enter failed", map[string]string{
						"agent_id": agentID,
						"error":    err.Error(),
					})
					return
				}
				time.Sleep(enterKeyDelay)
				if err := session.Write([]byte("\n")); err != nil {
					m.logger.Warn("agent prompt final enter failed", map[string]string{
						"agent_id": agentID,
						"error":    err.Error(),
					})
				}
			}
		}
	}()
}
