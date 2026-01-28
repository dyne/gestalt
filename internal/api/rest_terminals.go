package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/temporal/workflows"
	"gestalt/internal/terminal"
)

func (h *RestHandler) handleTerminals(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	switch r.Method {
	case http.MethodGet:
		return h.listTerminals(w)
	case http.MethodPost:
		return h.createTerminal(w, r)
	default:
		return methodNotAllowed(w, "GET, POST")
	}
}

func (h *RestHandler) handleTerminal(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	id, action, err := parseTerminalPath(r.URL.Path)
	if err != nil {
		return err
	}

	switch action {
	case terminalPathOutput:
		return h.handleTerminalOutput(w, r, id)
	case terminalPathHistory:
		return h.handleTerminalHistory(w, r, id)
	case terminalPathInputHistory:
		return h.handleTerminalInputHistory(w, r, id)
	case terminalPathBell:
		return h.handleTerminalBell(w, r, id)
	case terminalPathWorkflowResume:
		return h.handleTerminalWorkflowResume(w, r, id)
	case terminalPathWorkflowHistory:
		return h.handleTerminalWorkflowHistory(w, r, id)
	default:
		return h.handleTerminalDelete(w, r, id)
	}
}

func (h *RestHandler) listTerminals(w http.ResponseWriter) *apiError {
	infos := h.Manager.List()
	response := make([]terminalSummary, 0, len(infos))
	for _, info := range infos {
		response = append(response, terminalSummary{
			ID:          info.ID,
			Title:       info.Title,
			Role:        info.Role,
			CreatedAt:   info.CreatedAt,
			Status:      info.Status,
			LLMType:     info.LLMType,
			LLMModel:    info.LLMModel,
			Command:     info.Command,
			Skills:      info.Skills,
			PromptFiles: info.PromptFiles,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) createTerminal(w http.ResponseWriter, r *http.Request) *apiError {
	request, err := decodeCreateTerminalRequest(r)
	if err != nil {
		return err
	}

	if request.Agent != "" && h.Manager != nil {
		agentProfile, reloaded, loadErr := h.Manager.LoadAgentForSession(request.Agent)
		if loadErr != nil {
			if errors.Is(loadErr, terminal.ErrAgentNotFound) {
				return &apiError{Status: http.StatusBadRequest, Message: "unknown agent"}
			}
			return &apiError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("failed to refresh agent config: %s", loadErr.Error())}
		}
		if reloaded && h.Logger != nil && agentProfile != nil {
			h.Logger.Info("agent config reloaded for new session", map[string]string{
				"gestalt.category": "agent",
				"gestalt.source":   "backend",
				"agent.id":         request.Agent,
				"agent.name":       agentProfile.Name,
				"agent_id":         request.Agent,
				"agent_name":       agentProfile.Name,
				"hash":             agentProfile.ConfigHash,
			})
		}
	}

	session, createErr := h.Manager.CreateWithOptions(terminal.CreateOptions{
		AgentID:     request.Agent,
		Role:        request.Role,
		Title:       request.Title,
		UseWorkflow: request.Workflow,
	})
	if createErr != nil {
		if errors.Is(createErr, terminal.ErrAgentNotFound) {
			return &apiError{Status: http.StatusBadRequest, Message: "unknown agent"}
		}
		var dupErr *terminal.AgentAlreadyRunningError
		if errors.As(createErr, &dupErr) {
			return &apiError{
				Status:     http.StatusConflict,
				Message:    fmt.Sprintf("agent %q is already running", dupErr.AgentName),
				TerminalID: dupErr.TerminalID,
			}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to create terminal"}
	}

	info := session.Info()
	response := terminalSummary{
		ID:        info.ID,
		Title:     info.Title,
		Role:      info.Role,
		CreatedAt: info.CreatedAt,
		Status:    info.Status,
		LLMType:   info.LLMType,
		LLMModel:  info.LLMModel,
		Command:   info.Command,
		Skills:    info.Skills,
	}
	writeJSON(w, http.StatusCreated, response)
	return nil
}

func (h *RestHandler) handleTerminalOutput(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	response := terminalOutputResponse{
		ID:    id,
		Lines: session.OutputLines(),
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleTerminalHistory(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	lines, err := parseHistoryLines(r)
	if err != nil {
		return err
	}
	beforeCursor, err := parseHistoryBeforeCursor(r)
	if err != nil {
		return err
	}

	history, cursor, historyErr := h.Manager.HistoryPage(id, lines, beforeCursor)
	if historyErr != nil {
		if errors.Is(historyErr, terminal.ErrSessionNotFound) {
			return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read terminal history"}
	}

	response := terminalOutputResponse{
		ID:     id,
		Lines:  history,
		Cursor: cursor,
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleTerminalInputHistory(w http.ResponseWriter, r *http.Request, id string) *apiError {
	switch r.Method {
	case http.MethodGet:
		return h.handleTerminalInputHistoryGet(w, r, id)
	case http.MethodPost:
		return h.handleTerminalInputHistoryPost(w, r, id)
	default:
		return methodNotAllowed(w, "GET, POST")
	}
}

func (h *RestHandler) handleTerminalInputHistoryGet(w http.ResponseWriter, r *http.Request, id string) *apiError {
	limit, since, err := parseInputHistoryQuery(r)
	if err != nil {
		return err
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	entries := session.GetInputHistory()
	if since != nil {
		filtered := make([]terminal.InputEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.Timestamp.After(*since) || entry.Timestamp.Equal(*since) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	response := make([]inputHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		response = append(response, inputHistoryEntry{
			Command:   entry.Command,
			Timestamp: entry.Timestamp,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleTerminalInputHistoryPost(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Body == nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	var request inputHistoryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	command := strings.TrimSpace(request.Command)
	if command == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing command"}
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	session.RecordInput(command)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) handleTerminalBell(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	const bellContextLines = 50
	contextLines, historyError := session.HistoryLines(bellContextLines)
	if historyError != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read terminal history"}
	}
	contextText := strings.Join(contextLines, "\n")

	signalError := session.SendBellSignal(contextText)
	if signalError != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to signal terminal bell"}
	}

	if h.Logger != nil {
		h.Logger.Warn("terminal bell detected", map[string]string{
			"gestalt.category": "terminal",
			"gestalt.source":   "backend",
			"terminal.id":      id,
			"terminal_id":      id,
			"context_lines":    strconv.Itoa(len(contextLines)),
		})
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) handleTerminalWorkflowResume(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	request, err := decodeWorkflowResumeRequest(r)
	if err != nil {
		return err
	}
	action, err := normalizeWorkflowResumeAction(request)
	if err != nil {
		return err
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}
	workflowID, workflowRunID, hasWorkflow := session.WorkflowIdentifiers()
	if !hasWorkflow {
		return &apiError{Status: http.StatusConflict, Message: "workflow not active"}
	}

	if signalErr := session.SendResumeSignal(action); signalErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to resume workflow"}
	}

	if h.Logger != nil {
		h.Logger.Info("workflow resume action", map[string]string{
			"gestalt.category":    "workflow",
			"gestalt.source":      "backend",
			"terminal.id":         id,
			"terminal_id":         id,
			"workflow.id":         workflowID,
			"workflow.session_id": id,
			"workflow_id":         workflowID,
			"run_id":              workflowRunID,
			"action":              action,
		})
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) handleTerminalWorkflowHistory(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	if err := h.requireManager(); err != nil {
		return err
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}
	workflowID, workflowRunID, hasWorkflow := session.WorkflowIdentifiers()
	if !hasWorkflow {
		return &apiError{Status: http.StatusConflict, Message: "workflow not active"}
	}

	temporalClient := h.Manager.TemporalClient()
	events, err := fetchWorkflowHistoryEntries(r.Context(), temporalClient, workflowID, workflowRunID, h.Logger)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to load workflow history"}
	}

	writeJSON(w, http.StatusOK, events)
	return nil
}

func (h *RestHandler) handleTerminalDelete(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodDelete {
		return methodNotAllowed(w, "DELETE")
	}

	if err := h.Manager.Delete(id); err != nil {
		if err == terminal.ErrSessionNotFound {
			return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to delete terminal"}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func parseTerminalPath(path string) (string, terminalPathAction, *apiError) {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", terminalPathTerminal, &apiError{Status: http.StatusBadRequest, Message: "missing terminal id"}
	}

	parts := strings.Split(trimmed, "/")
	id := parts[0]
	if err := validateTerminalID(id); err != nil {
		return "", terminalPathTerminal, err
	}

	switch len(parts) {
	case 1:
		return id, terminalPathTerminal, nil
	case 2:
		switch parts[1] {
		case "output":
			return id, terminalPathOutput, nil
		case "history":
			return id, terminalPathHistory, nil
		case "input-history":
			return id, terminalPathInputHistory, nil
		case "bell":
			return id, terminalPathBell, nil
		default:
			return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
	case 3:
		if parts[1] != "workflow" {
			return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
		switch parts[2] {
		case "resume":
			return id, terminalPathWorkflowResume, nil
		case "history":
			return id, terminalPathWorkflowHistory, nil
		default:
			return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
	default:
		return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}
}

func parseHistoryLines(r *http.Request) (int, *apiError) {
	lines := terminal.DefaultHistoryLines
	if rawLines := strings.TrimSpace(r.URL.Query().Get("lines")); rawLines != "" {
		parsed, err := strconv.Atoi(rawLines)
		if err != nil || parsed <= 0 {
			return lines, &apiError{Status: http.StatusBadRequest, Message: "invalid lines"}
		}
		lines = parsed
	}
	return lines, nil
}

func parseHistoryBeforeCursor(r *http.Request) (*int64, *apiError) {
	rawCursor := strings.TrimSpace(r.URL.Query().Get("before_cursor"))
	if rawCursor == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(rawCursor, 10, 64)
	if err != nil || parsed < 0 {
		return nil, &apiError{Status: http.StatusBadRequest, Message: "invalid before_cursor"}
	}
	return &parsed, nil
}

func parseInputHistoryQuery(r *http.Request) (int, *time.Time, *apiError) {
	limit := 100
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			return limit, nil, &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
		}
		limit = parsed
	}

	if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
		parsed, err := time.Parse(time.RFC3339, rawSince)
		if err != nil {
			return limit, nil, &apiError{Status: http.StatusBadRequest, Message: "invalid since timestamp"}
		}
		return limit, &parsed, nil
	}

	return limit, nil, nil
}

func validateTerminalID(id string) *apiError {
	if strings.TrimSpace(id) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing terminal id"}
	}
	return nil
}

func decodeCreateTerminalRequest(r *http.Request) (createTerminalRequest, *apiError) {
	var request createTerminalRequest
	if r.Body == nil {
		return request, nil
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return request, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	return request, nil
}

func decodeWorkflowResumeRequest(r *http.Request) (workflowResumeRequest, *apiError) {
	var request workflowResumeRequest
	if r.Body == nil {
		return request, nil
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return request, &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	return request, nil
}

func normalizeWorkflowResumeAction(request workflowResumeRequest) (string, *apiError) {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action == "" {
		return workflows.ResumeActionContinue, nil
	}
	switch action {
	case workflows.ResumeActionContinue, workflows.ResumeActionAbort, workflows.ResumeActionHandoff:
		return action, nil
	default:
		return "", &apiError{Status: http.StatusBadRequest, Message: "invalid resume action"}
	}
}
