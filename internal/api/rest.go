package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

type RestHandler struct {
	Manager  *terminal.Manager
	Logger   *logging.Logger
	PlanPath string
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

type inputHistoryEntry struct {
	Command   string    `json:"command"`
	Timestamp time.Time `json:"timestamp"`
}

type inputHistoryRequest struct {
	Command string `json:"command"`
}

type statusResponse struct {
	TerminalCount  int       `json:"terminal_count"`
	ServerTime     time.Time `json:"server_time"`
	SessionPersist bool      `json:"session_persist"`
}

type planResponse struct {
	Content string `json:"content"`
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

type skillSummary struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	License       string `json:"license"`
	HasScripts    bool   `json:"has_scripts"`
	HasReferences bool   `json:"has_references"`
	HasAssets     bool   `json:"has_assets"`
}

type skillDetail struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	License       string         `json:"license"`
	Compatibility string         `json:"compatibility"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	AllowedTools  []string       `json:"allowed_tools,omitempty"`
	Path          string         `json:"path"`
	Content       string         `json:"content"`
	Scripts       []string       `json:"scripts"`
	References    []string       `json:"references"`
	Assets        []string       `json:"assets"`
}

type logQuery struct {
	Limit int
	Level logging.Level
	Since *time.Time
}

type clientLogRequest struct {
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Context map[string]string `json:"context"`
}

type createTerminalRequest struct {
	Title string `json:"title"`
	Role  string `json:"role"`
	Agent string `json:"agent"`
}

type terminalPathAction int

const (
	terminalPathTerminal terminalPathAction = iota
	terminalPathOutput
	terminalPathHistory
	terminalPathInputHistory
)

func (h *RestHandler) handleStatus(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	terminals := h.Manager.List()
	response := statusResponse{
		TerminalCount:  len(terminals),
		ServerTime:     time.Now().UTC(),
		SessionPersist: h.Manager.SessionPersistenceEnabled(),
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

	id, action := parseTerminalPath(r.URL.Path)
	if err := validateTerminalID(id); err != nil {
		return err
	}

	switch action {
	case terminalPathOutput:
		return h.handleTerminalOutput(w, r, id)
	case terminalPathHistory:
		return h.handleTerminalHistory(w, r, id)
	case terminalPathInputHistory:
		return h.handleTerminalInputHistory(w, r, id)
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

func (h *RestHandler) handleSkills(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	agentID := strings.TrimSpace(r.URL.Query().Get("agent"))
	metas := h.Manager.ListSkills()
	if agentID != "" {
		agentProfile, ok := h.Manager.GetAgent(agentID)
		if !ok {
			return &apiError{Status: http.StatusNotFound, Message: "agent not found"}
		}
		byName := make(map[string]terminal.SkillMetadata, len(metas))
		for _, meta := range metas {
			byName[meta.Name] = meta
		}
		filtered := make([]terminal.SkillMetadata, 0, len(agentProfile.Skills))
		for _, name := range agentProfile.Skills {
			if meta, ok := byName[name]; ok {
				filtered = append(filtered, meta)
			}
		}
		metas = filtered
	}

	response := make([]skillSummary, 0, len(metas))
	for _, meta := range metas {
		response = append(response, skillSummary{
			Name:          meta.Name,
			Description:   meta.Description,
			Path:          meta.Path,
			License:       meta.License,
			HasScripts:    hasSkillDir(meta.Path, "scripts"),
			HasReferences: hasSkillDir(meta.Path, "references"),
			HasAssets:     hasSkillDir(meta.Path, "assets"),
		})
	}

	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleSkill(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	name = strings.TrimSuffix(name, "/")
	if strings.TrimSpace(name) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing skill name"}
	}

	entry, ok := h.Manager.GetSkill(name)
	if !ok || entry == nil {
		return &apiError{Status: http.StatusNotFound, Message: "skill not found"}
	}

	response := skillDetail{
		Name:          entry.Name,
		Description:   entry.Description,
		License:       entry.License,
		Compatibility: entry.Compatibility,
		Metadata:      entry.Metadata,
		AllowedTools:  entry.AllowedTools,
		Path:          entry.Path,
		Content:       entry.Content,
		Scripts:       listSkillFiles(entry.Path, "scripts"),
		References:    listSkillFiles(entry.Path, "references"),
		Assets:        listSkillFiles(entry.Path, "assets"),
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleLogs(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireLogger(); err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		query, err := parseLogQuery(r)
		if err != nil {
			return err
		}

		entries := h.Logger.Buffer().List()
		filtered := filterLogEntries(entries, query)
		writeJSON(w, http.StatusOK, filtered)
		return nil
	case http.MethodPost:
		return h.createLogEntry(w, r)
	default:
		return methodNotAllowed(w, "GET, POST")
	}
}

func (h *RestHandler) handlePlan(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	planPath := h.PlanPath
	if planPath == "" {
		planPath = "PLAN.org"
	}

	info, statErr := os.Stat(planPath)
	content, err := os.ReadFile(planPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if h.Logger != nil {
				h.Logger.Warn("plan file not found", map[string]string{
					"path": planPath,
				})
			}
			writeJSON(w, http.StatusOK, planResponse{Content: ""})
			return nil
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read plan file"}
	}
	if statErr == nil {
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	}

	writeJSON(w, http.StatusOK, planResponse{Content: string(content)})
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

func (h *RestHandler) handleTerminalHistory(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	lines, err := parseHistoryLines(r)
	if err != nil {
		return err
	}

	history, historyErr := h.Manager.HistoryLines(id, lines)
	if historyErr != nil {
		if errors.Is(historyErr, terminal.ErrSessionNotFound) {
			return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to read terminal history"}
	}

	response := terminalOutputResponse{
		ID:    id,
		Lines: history,
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

func (h *RestHandler) createLogEntry(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Body == nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	var request clientLogRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && err != io.EOF {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	message := strings.TrimSpace(request.Message)
	if message == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing log message"}
	}

	level := logging.LevelInfo
	if rawLevel := strings.TrimSpace(request.Level); rawLevel != "" {
		parsed, ok := logging.ParseLevel(rawLevel)
		if !ok {
			return &apiError{Status: http.StatusBadRequest, Message: "invalid log level"}
		}
		level = parsed
	}

	fields := make(map[string]string, len(request.Context)+2)
	for key, value := range request.Context {
		if strings.TrimSpace(key) == "" {
			continue
		}
		fields[key] = value
	}
	if _, ok := fields["source"]; !ok {
		fields["source"] = "frontend"
	}
	if _, ok := fields["toast"]; !ok {
		fields["toast"] = "true"
	}

	switch level {
	case logging.LevelDebug:
		h.Logger.Debug(message, fields)
	case logging.LevelWarning:
		h.Logger.Warn(message, fields)
	case logging.LevelError:
		h.Logger.Error(message, fields)
	default:
		h.Logger.Info(message, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func parseTerminalPath(path string) (string, terminalPathAction) {
	trimmed := strings.TrimPrefix(path, "/api/terminals/")
	if trimmed == path {
		return "", terminalPathTerminal
	}

	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", terminalPathTerminal
	}

	if strings.HasSuffix(trimmed, "/output") {
		id := strings.TrimSuffix(trimmed, "/output")
		return id, terminalPathOutput
	}
	if strings.HasSuffix(trimmed, "/history") {
		id := strings.TrimSuffix(trimmed, "/history")
		return id, terminalPathHistory
	}
	if strings.HasSuffix(trimmed, "/input-history") {
		id := strings.TrimSuffix(trimmed, "/input-history")
		return id, terminalPathInputHistory
	}
	return trimmed, terminalPathTerminal
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

func hasSkillDir(base, name string) bool {
	if strings.TrimSpace(base) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(base, name))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func listSkillFiles(base, name string) []string {
	if strings.TrimSpace(base) == "" {
		return nil
	}
	path := filepath.Join(base, name)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
