package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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
		terminalID, running := h.Manager.GetAgentTerminal(info.Name)
		response = append(response, agentSummary{
			ID:          info.ID,
			Name:        info.Name,
			LLMType:     info.LLMType,
			LLMModel:    info.LLMModel,
			TerminalID:  terminalID,
			Running:     running,
			UseWorkflow: info.UseWorkflow,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleAgentInput(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	agentName, err := parseAgentInputPath(r.URL.Path)
	if err != nil {
		return err
	}

	session, ok := h.Manager.GetSessionByAgent(agentName)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: fmt.Sprintf("agent %q is not running", agentName)}
	}

	payload, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	if writeErr := session.Write(payload); writeErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to send agent input"}
	}

	writeJSON(w, http.StatusOK, agentInputResponse{Bytes: len(payload)})
	return nil
}

func parseAgentInputPath(path string) (string, *apiError) {
	trimmed := strings.TrimSuffix(path, "/")
	const prefix = "/api/agents/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", &apiError{Status: http.StatusNotFound, Message: "agent not found"}
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "input" {
		return "", &apiError{Status: http.StatusNotFound, Message: "agent not found"}
	}
	agentName := parts[0]
	if strings.TrimSpace(agentName) == "" {
		return "", &apiError{Status: http.StatusBadRequest, Message: "missing agent name"}
	}
	return agentName, nil
}
