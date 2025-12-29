package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type RestHandler struct {
	Manager *terminal.Manager
	Logger  *logging.Logger
}

type terminalSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
	LLMType   string    `json:"llm_type"`
	LLMModel  string    `json:"llm_model"`
}

type terminalOutputResponse struct {
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
}

type statusResponse struct {
	TerminalCount int       `json:"terminal_count"`
	ServerTime    time.Time `json:"server_time"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type agentSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	LLMType  string `json:"llm_type"`
	LLMModel string `json:"llm_model"`
}

type logQuery struct {
	Limit int
	Level logging.Level
	Since *time.Time
}

type createTerminalRequest struct {
	Title string `json:"title"`
	Role  string `json:"role"`
	Agent string `json:"agent"`
}

func (h *RestHandler) handleStatus(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	terminals := h.Manager.List()
	response := statusResponse{
		TerminalCount: len(terminals),
		ServerTime:    time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, response)
	return nil
}

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

	id, wantsOutput := parseTerminalPath(r.URL.Path)
	if err := validateTerminalID(id); err != nil {
		return err
	}

	if wantsOutput {
		return h.handleTerminalOutput(w, r, id)
	}

	return h.handleTerminalDelete(w, r, id)
}

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
		response = append(response, agentSummary{
			ID:       info.ID,
			Name:     info.Name,
			LLMType:  info.LLMType,
			LLMModel: info.LLMModel,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleLogs(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireLogger(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	query, err := parseLogQuery(r)
	if err != nil {
		return err
	}

	entries := h.Logger.Buffer().List()
	filtered := filterLogEntries(entries, query)
	writeJSON(w, http.StatusOK, filtered)
	return nil
}

func (h *RestHandler) requireManager() *apiError {
	if h.Manager == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "terminal manager unavailable"}
	}
	return nil
}

func (h *RestHandler) requireLogger() *apiError {
	if h.Logger == nil || h.Logger.Buffer() == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "log buffer unavailable"}
	}
	return nil
}

func (h *RestHandler) listTerminals(w http.ResponseWriter) *apiError {
	infos := h.Manager.List()
	response := make([]terminalSummary, 0, len(infos))
	for _, info := range infos {
		response = append(response, terminalSummary{
			ID:        info.ID,
			Title:     info.Title,
			Role:      info.Role,
			CreatedAt: info.CreatedAt,
			Status:    info.Status,
			LLMType:   info.LLMType,
			LLMModel:  info.LLMModel,
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

	session, createErr := h.Manager.Create(request.Agent, request.Role, request.Title)
	if createErr != nil {
		if errors.Is(createErr, terminal.ErrAgentNotFound) {
			return &apiError{Status: http.StatusBadRequest, Message: "unknown agent"}
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

func parseTerminalPath(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return "", false
	}

	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", false
	}

	if strings.HasSuffix(trimmed, "/output") {
		id := strings.TrimSuffix(trimmed, "/output")
		return id, true
	}
	return trimmed, false
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

func parseLogQuery(r *http.Request) (logQuery, *apiError) {
	values := r.URL.Query()
	query := logQuery{
		Limit: 100,
	}

	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
		}
		query.Limit = limit
	}

	if rawSince := strings.TrimSpace(values.Get("since")); rawSince != "" {
		parsed, err := time.Parse(time.RFC3339, rawSince)
		if err != nil {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid since timestamp"}
		}
		query.Since = &parsed
	}

	if rawLevel := strings.TrimSpace(values.Get("level")); rawLevel != "" {
		level, ok := logging.ParseLevel(rawLevel)
		if !ok {
			return query, &apiError{Status: http.StatusBadRequest, Message: "invalid log level"}
		}
		query.Level = level
	}

	return query, nil
}

func filterLogEntries(entries []logging.LogEntry, query logQuery) []logging.LogEntry {
	filtered := make([]logging.LogEntry, 0, len(entries))
	for _, entry := range entries {
		if query.Level != "" && !logging.LevelAtLeast(entry.Level, query.Level) {
			continue
		}
		if query.Since != nil && entry.Timestamp.Before(*query.Since) {
			continue
		}
		filtered = append(filtered, entry)
	}

	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[len(filtered)-query.Limit:]
	}

	return filtered
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
