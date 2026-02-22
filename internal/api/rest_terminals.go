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

	"gestalt/internal/event"
	"gestalt/internal/flow"
	"gestalt/internal/notify"
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
	case terminalPathInput:
		return h.handleTerminalInput(w, r, id)
	case terminalPathActivate:
		return h.handleTerminalActivate(w, r, id)
	case terminalPathInputHistory:
		return h.handleTerminalInputHistory(w, r, id)
	case terminalPathBell:
		return h.handleTerminalBell(w, r, id)
	case terminalPathNotify:
		return h.handleTerminalNotify(w, r, id)
	case terminalPathProgress:
		return h.handleTerminalProgress(w, r, id)
	default:
		return h.handleTerminalDelete(w, r, id)
	}
}

func (h *RestHandler) handleTerminalInput(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	if len(payload) == 0 {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	if session.IsMCP() {
		payload = normalizeMCPInput(payload)
	}
	if writeErr := session.Write(payload); writeErr != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to write terminal input"}
	}

	writeJSON(w, http.StatusOK, agentInputResponse{Bytes: len(payload)})
	return nil
}

func (h *RestHandler) handleTerminalActivate(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	if err := h.Manager.ActivateSessionWindow(id); err != nil {
		if errors.Is(err, terminal.ErrSessionNotFound) {
			return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
		if errors.Is(err, terminal.ErrSessionNotTmuxManaged) {
			return &apiError{Status: http.StatusConflict, Message: "session is not tmux-managed"}
		}
		if errors.Is(err, terminal.ErrTmuxSessionNotFound) {
			return &apiError{Status: http.StatusServiceUnavailable, Message: "tmux session not found; start an agent to recreate it"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to activate tmux window"}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) listTerminals(w http.ResponseWriter) *apiError {
	h.Manager.PruneMissingExternalTmuxSessions()
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
			Model:       info.Model,
			Interface:   info.Interface,
			Runner:      info.Runner,
			Command:     info.Command,
			Skills:      info.Skills,
			PromptFiles: info.PromptFiles,
			GUIModules:  info.GUIModules,
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
		AgentID:    request.Agent,
		Role:       request.Role,
		Title:      request.Title,
		Runner:     request.Runner,
		GUIModules: request.GUIModules,
	})
	if createErr != nil {
		if errors.Is(createErr, terminal.ErrAgentRequired) {
			return &apiError{Status: http.StatusBadRequest, Message: "agent is required"}
		}
		if errors.Is(createErr, terminal.ErrAgentNotFound) {
			return &apiError{Status: http.StatusBadRequest, Message: "unknown agent"}
		}
		if errors.Is(createErr, terminal.ErrCodexMCPBootstrap) {
			return &apiError{
				Status:  http.StatusInternalServerError,
				Message: "failed to start MCP session runtime",
				Code:    "mcp_bootstrap_failed",
			}
		}
		var tmuxErr *terminal.ExternalTmuxError
		if errors.As(createErr, &tmuxErr) {
			return &apiError{Status: http.StatusInternalServerError, Message: tmuxErr.Message}
		}
		var dupErr *terminal.AgentAlreadyRunningError
		if errors.As(createErr, &dupErr) {
			return &apiError{
				Status:    http.StatusConflict,
				Message:   fmt.Sprintf("agent %q is already running", dupErr.AgentName),
				SessionID: dupErr.TerminalID,
			}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to create terminal"}
	}

	info := session.Info()
	response := terminalCreateResponse{
		terminalSummary: terminalSummary{
			ID:          info.ID,
			Title:       info.Title,
			Role:        info.Role,
			CreatedAt:   info.CreatedAt,
			Status:      info.Status,
			LLMType:     info.LLMType,
			Model:       info.Model,
			Interface:   info.Interface,
			Runner:      info.Runner,
			Command:     info.Command,
			Skills:      info.Skills,
			PromptFiles: info.PromptFiles,
			GUIModules:  info.GUIModules,
		},
	}
	if session.LaunchSpec != nil {
		response.Launch = session.LaunchSpec
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
			"session.id":       id,
			"session_id":       id,
			"context_lines":    strconv.Itoa(len(contextLines)),
		})
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) handleTerminalNotify(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, "POST")
	}

	request, err := decodeNotifyRequest(r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.SessionID) != id {
		return &apiError{Status: http.StatusBadRequest, Message: "session id mismatch"}
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "session not found"}
	}
	agentID := strings.TrimSpace(session.AgentID)
	if agentID == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "terminal is not an agent session"}
	}

	isProgress := request.EventType == "progress" || request.EventType == "plan-update"
	notifyTime := time.Now().UTC()
	if request.OccurredAt != nil && !request.OccurredAt.IsZero() {
		notifyTime = request.OccurredAt.UTC()
	}
	if request.EventType == "progress" || request.EventType == "plan-update" {
		progressPayload, normalized, normalizeErr := normalizePlanProgressPayload(request.Payload)
		if normalizeErr != nil {
			return normalizeErr
		}
		request.Payload = normalized
		session.SetPlanProgress(terminal.PlanProgress{
			PlanFile:  progressPayload.PlanFile,
			L1:        progressPayload.L1,
			L2:        progressPayload.L2,
			TaskLevel: progressPayload.TaskLevel,
			TaskState: progressPayload.TaskState,
			UpdatedAt: notifyTime,
		})
		if bus := h.Manager.TerminalBus(); bus != nil {
			terminalEvent := event.NewTerminalEvent(id, "plan-update")
			terminalEvent.OccurredAt = notifyTime
			terminalEvent.Data = map[string]any{
				"plan_file":  progressPayload.PlanFile,
				"l1":         progressPayload.L1,
				"l2":         progressPayload.L2,
				"task_level": progressPayload.TaskLevel,
				"task_state": progressPayload.TaskState,
				"timestamp":  notifyTime,
			}
			bus.Publish(terminalEvent)
		}
	}

	if request.EventType == "prompt-text" || request.EventType == "prompt-voice" {
		var payload map[string]any
		if err := json.Unmarshal(request.Payload, &payload); err != nil || payload == nil {
			return &apiError{Status: http.StatusUnprocessableEntity, Message: "payload must be a JSON object"}
		}
		updated := false
		if _, ok := payload["git_branch"]; !ok {
			_, branch := h.gitInfo()
			if strings.TrimSpace(branch) != "" {
				payload["git_branch"] = branch
				updated = true
			}
		}
		if _, ok := payload["plan_file"]; !ok {
			if progress, ok := session.PlanProgress(); ok && strings.TrimSpace(progress.PlanFile) != "" {
				payload["plan_file"] = progress.PlanFile
				updated = true
			}
		}
		if updated {
			normalized, err := json.Marshal(payload)
			if err != nil {
				return &apiError{Status: http.StatusInternalServerError, Message: "failed to normalize prompt payload"}
			}
			request.Payload = normalized
		}
	}

	fields, fieldsErr := buildNotifyFlowFields(session, request, notifyTime)
	if fieldsErr != nil {
		return fieldsErr
	}

	logFields := buildNotifyLogFields(fields, request)
	if h.Logger != nil && (request.EventType == "prompt-text" || request.EventType == "prompt-voice") {
		h.Logger.Debug("prompt input sent", logFields)
	}
	dispatch := "failed"
	defer func() {
		if h.Logger == nil {
			return
		}
		logFields["notify.dispatch"] = dispatch
		h.Logger.Info("notify event accepted", logFields)
	}()

	if h.NotificationSink == nil {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "notification sink unavailable"}
	}
	sinkEvent := notify.Event{
		Fields:     fields,
		OccurredAt: notifyTime,
		Level:      "info",
		Message:    request.EventType,
	}
	if err := h.NotificationSink.Emit(r.Context(), sinkEvent); err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to emit notification"}
	}

	if err := h.requireFlowService(); err != nil {
		if isProgress {
			dispatch = "flow_unavailable"
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return err
	}

	if signalErr := h.FlowService.SignalEvent(r.Context(), fields, request.EventID); signalErr != nil {
		if errors.Is(signalErr, flow.ErrDispatcherUnavailable) {
			if isProgress {
				dispatch = "flow_unavailable"
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			return &apiError{Status: http.StatusServiceUnavailable, Message: "flow dispatcher unavailable"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to dispatch flow activity"}
	}

	dispatch = "queued"
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *RestHandler) handleTerminalProgress(w http.ResponseWriter, r *http.Request, id string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	session, ok := h.Manager.Get(id)
	if !ok {
		return &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
	}

	progress, ok := session.PlanProgress()
	if !ok {
		writeJSON(w, http.StatusOK, terminalProgressResponse{HasProgress: false})
		return nil
	}

	updatedAt := progress.UpdatedAt
	response := terminalProgressResponse{
		HasProgress: true,
		PlanFile:    progress.PlanFile,
		L1:          progress.L1,
		L2:          progress.L2,
		TaskLevel:   progress.TaskLevel,
		TaskState:   progress.TaskState,
		UpdatedAt:   &updatedAt,
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

func parseTerminalPath(path string) (string, terminalPathAction, *apiError) {
	trimmed := strings.TrimPrefix(path, "/api/sessions/")
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
		case "input":
			return id, terminalPathInput, nil
		case "activate":
			return id, terminalPathActivate, nil
		case "bell":
			return id, terminalPathBell, nil
		case "notify":
			return id, terminalPathNotify, nil
		case "progress":
			return id, terminalPathProgress, nil
		default:
			return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
		}
	case 3:
		return "", terminalPathTerminal, &apiError{Status: http.StatusNotFound, Message: "terminal not found"}
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
