package api

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		sessionID, running := h.Manager.GetAgentTerminal(info.Name)
		response = append(response, agentSummary{
			ID:        info.ID,
			Name:      info.Name,
			LLMType:   info.LLMType,
			Model:     info.Model,
			Interface: info.Interface,
			SessionID: sessionID,
			Running:   running,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleAgentSendInput(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	agentName, err := parseAgentSendInputPath(r.URL.Path)
	if err != nil {
		return err
	}

	var payload agentSendInputRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decodeErr := decoder.Decode(&payload); decodeErr != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	input := strings.TrimSpace(payload.Input)
	if input == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "input is required"}
	}

	terminalID, ok := h.lookupAgentTerminal(agentName)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: fmt.Sprintf("agent %q is not running", agentName)}
	}
	session, ok := h.Manager.Get(terminalID)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: fmt.Sprintf("agent %q is not running", agentName)}
	}

	if session.IsMCP() {
		if writeErr := session.Write(normalizeMCPInput([]byte(input))); writeErr != nil {
			return &apiError{Status: http.StatusInternalServerError, Message: "failed to send agent input"}
		}
	} else if writeErr := session.Write([]byte(input + "\n")); writeErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to send agent input"}
	}

	writeJSON(w, http.StatusOK, struct{}{})
	return nil
}

func parseAgentSendInputPath(path string) (string, *apiError) {
	trimmed := strings.TrimSuffix(path, "/")
	const prefix = "/api/agents/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", &apiError{Status: http.StatusNotFound, Message: "agent not found"}
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "send-input" {
		return "", &apiError{Status: http.StatusNotFound, Message: "agent not found"}
	}
	agentName := parts[0]
	if strings.TrimSpace(agentName) == "" {
		return "", &apiError{Status: http.StatusBadRequest, Message: "missing agent name"}
	}
	return agentName, nil
}

type agentSendInputRequest struct {
	Input string `json:"input"`
}

func (h *RestHandler) lookupAgentTerminal(agentName string) (string, bool) {
	if h == nil || h.Manager == nil {
		return "", false
	}
	if terminalID, ok := h.Manager.GetAgentTerminal(agentName); ok {
		return terminalID, true
	}
	for _, info := range h.Manager.ListAgents() {
		if strings.EqualFold(info.Name, agentName) {
			if terminalID, ok := h.Manager.GetAgentTerminal(info.Name); ok {
				return terminalID, true
			}
		}
	}
	for _, info := range h.Manager.List() {
		if strings.EqualFold(info.Role, agentName) {
			return info.ID, true
		}
	}
	return "", false
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
