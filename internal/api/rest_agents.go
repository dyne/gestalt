package api

import (
	"bytes"
	"net/http"
)

func (h *RestHandler) handleAgents(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	infos := h.Manager.ListAgents()
	response := make([]agentSummary, 0, len(infos))
	for _, info := range infos {
		sessionID, running := h.Manager.GetAgentTerminal(info.Name)
		response = append(response, agentSummary{
			ID:        info.ID,
			Name:      info.Name,
			LLMType:   info.LLMType,
			Model:     info.Model,
			Interface: info.Interface,
			SessionID: sessionID,
			Running:   running,
			Hidden:    info.Hidden,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func normalizeMCPInput(input []byte) []byte {
	if len(input) == 0 {
		return input
	}
	trimmed := bytes.TrimRight(input, "\r\n")
	if len(trimmed) == 0 {
		return []byte{'\r'}
	}
	return append(trimmed, '\r')
}
