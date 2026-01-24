package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/metrics"
	"gestalt/internal/plan"
	"gestalt/internal/temporal"
	"gestalt/internal/temporal/workflows"
	"gestalt/internal/terminal"
	"gestalt/internal/version"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/converter"
)

type RestHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	PlanPath       string
	PlanCache      *plan.Cache
	GitOrigin      string
	GitBranch      string
	TemporalUIPort int
	gitMutex       sync.RWMutex
}

type terminalSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
	LLMType     string    `json:"llm_type"`
	LLMModel    string    `json:"llm_model"`
	Command     string    `json:"command,omitempty"`
	Skills      []string  `json:"skills"`
	PromptFiles []string  `json:"prompt_files"`
}

type workflowSummary struct {
	SessionID     string              `json:"session_id"`
	WorkflowID    string              `json:"workflow_id"`
	WorkflowRunID string              `json:"workflow_run_id"`
	Title         string              `json:"title"`
	Role          string              `json:"role"`
	AgentName     string              `json:"agent_name"`
	CurrentL1     string              `json:"current_l1"`
	CurrentL2     string              `json:"current_l2"`
	Status        string              `json:"status"`
	StartTime     time.Time           `json:"start_time"`
	BellEvents    []workflowBellEvent `json:"bell_events"`
	TaskEvents    []workflowTaskEvent `json:"task_events"`
}

type workflowBellEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Context   string    `json:"context"`
}

type workflowTaskEvent struct {
	Timestamp time.Time `json:"timestamp"`
	L1        string    `json:"l1"`
	L2        string    `json:"l2"`
}

type workflowHistoryEntry struct {
	EventID    int64     `json:"event_id"`
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	SignalName string    `json:"signal_name,omitempty"`
	Action     string    `json:"action,omitempty"`
	L1         string    `json:"l1,omitempty"`
	L2         string    `json:"l2,omitempty"`
	Context    string    `json:"context,omitempty"`
	Reason     string    `json:"reason,omitempty"`
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
	WorkingDir     string    `json:"working_dir"`
	GitOrigin      string    `json:"git_origin"`
	GitBranch      string    `json:"git_branch"`
	Version        string    `json:"version"`
	Major          int       `json:"major"`
	Minor          int       `json:"minor"`
	Patch          int       `json:"patch"`
	Built          string    `json:"built"`
	GitCommit      string    `json:"git_commit,omitempty"`
	TemporalUIURL  string    `json:"temporal_ui_url,omitempty"`
}

type eventBusDebug struct {
	Name                  string `json:"name"`
	FilteredSubscribers   int64  `json:"filtered_subscribers"`
	UnfilteredSubscribers int64  `json:"unfiltered_subscribers"`
}

type planResponse struct {
	Content string `json:"content"`
}

type planCurrentResponse struct {
	L1 string `json:"l1"`
	L2 string `json:"l2"`
}

type errorResponse struct {
	Error      string `json:"error"`
	TerminalID string `json:"terminal_id,omitempty"`
}

type agentSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LLMType     string `json:"llm_type"`
	LLMModel    string `json:"llm_model"`
	TerminalID  string `json:"terminal_id"`
	Running     bool   `json:"running"`
	UseWorkflow bool   `json:"use_workflow"`
}

type agentInputResponse struct {
	Bytes int `json:"bytes"`
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
	Title    string `json:"title"`
	Role     string `json:"role"`
	Agent    string `json:"agent"`
	Workflow *bool  `json:"workflow,omitempty"`
}

type workflowResumeRequest struct {
	Action string `json:"action"`
}

type terminalPathAction int

const (
	terminalPathTerminal terminalPathAction = iota
	terminalPathOutput
	terminalPathHistory
	terminalPathInputHistory
	terminalPathBell
	terminalPathWorkflowResume
	terminalPathWorkflowHistory
)

const workflowQueryTimeout = 3 * time.Second
const workflowStatusUnknown = "unknown"
const workflowHistoryTimeout = 5 * time.Second

func (h *RestHandler) handleStatus(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}

	workDir, err := os.Getwd()
	if err != nil {
		workDir = "unknown"
		if h.Logger != nil {
			h.Logger.Warn("failed to get working directory", map[string]string{
				"error": err.Error(),
			})
		}
	}

	terminals := h.Manager.List()
	gitOrigin, gitBranch := h.gitInfo()
	versionInfo := version.GetVersionInfo()
	response := statusResponse{
		TerminalCount:  len(terminals),
		ServerTime:     time.Now().UTC(),
		SessionPersist: h.Manager.SessionPersistenceEnabled(),
		WorkingDir:     workDir,
		GitOrigin:      gitOrigin,
		GitBranch:      gitBranch,
		Version:        versionInfo.Version,
		Major:          versionInfo.Major,
		Minor:          versionInfo.Minor,
		Patch:          versionInfo.Patch,
		Built:          versionInfo.Built,
		GitCommit:      versionInfo.GitCommit,
		TemporalUIURL:  buildTemporalUIURL(r, h.TemporalUIPort),
	}

	writeJSON(w, http.StatusOK, response)
	return nil
}

func buildTemporalUIURL(r *http.Request, uiPort int) string {
	if uiPort <= 0 || r == nil {
		return ""
	}
	host := forwardedHeaderValue(r, "X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if idx := strings.Index(host, ","); idx >= 0 {
		host = strings.TrimSpace(host[:idx])
	}
	hostname := host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		hostname = splitHost
	}
	if strings.TrimSpace(hostname) == "" {
		return ""
	}
	scheme := forwardedHeaderValue(r, "X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(hostname, strconv.Itoa(uiPort)))
}

func forwardedHeaderValue(r *http.Request, header string) string {
	if r == nil {
		return ""
	}
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}

func (h *RestHandler) handleMetrics(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := metrics.Default.WritePrometheus(w); err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to write metrics"}
	}
	return nil
}

func (h *RestHandler) handleEventDebug(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	snapshots := metrics.Default.EventBusSnapshots()
	response := make([]eventBusDebug, 0, len(snapshots))
	for _, snapshot := range snapshots {
		response = append(response, eventBusDebug{
			Name:                  snapshot.Name,
			FilteredSubscribers:   snapshot.FilteredSubscribers,
			UnfilteredSubscribers: snapshot.UnfilteredSubscribers,
		})
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}

func (h *RestHandler) handleWorkflows(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.requireManager(); err != nil {
		return err
	}
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if !h.Manager.TemporalEnabled() || h.Manager.TemporalClient() == nil {
		writeJSON(w, http.StatusOK, []workflowSummary{})
		return nil
	}

	summaries := h.listWorkflowSummaries(r.Context())
	writeJSON(w, http.StatusOK, summaries)
	return nil
}

func (h *RestHandler) setGitBranch(branch string) {
	if h == nil {
		return
	}
	h.gitMutex.Lock()
	h.GitBranch = branch
	h.gitMutex.Unlock()
}

func (h *RestHandler) gitInfo() (string, string) {
	if h == nil {
		return "", ""
	}
	h.gitMutex.RLock()
	origin := h.GitOrigin
	branch := h.GitBranch
	h.gitMutex.RUnlock()
	return origin, branch
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
	case terminalPathBell:
		return h.handleTerminalBell(w, r, id)
	case terminalPathWorkflowResume:
		return h.handleTerminalWorkflowResume(w, r, id)
	case terminalPathWorkflowHistory:
		return h.handleTerminalWorkflowHistory(w, r, id)
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
		planPath = plan.DefaultPath()
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
			content = []byte{}
		} else {
			return &apiError{Status: http.StatusInternalServerError, Message: "failed to read plan file"}
		}
	}
	if statErr == nil {
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	}

	etag := planETag(content)
	w.Header().Set("ETag", etag)
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	writeJSON(w, http.StatusOK, planResponse{Content: string(content)})
	return nil
}

func (h *RestHandler) handlePlanCurrent(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if h.PlanCache == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "plan cache unavailable"}
	}

	currentWork, currentError := h.PlanCache.Current()
	if currentError != nil {
		if h.Logger != nil {
			h.Logger.Warn("plan current read failed", map[string]string{
				"error": currentError.Error(),
			})
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to parse plan file"}
	}

	writeJSON(w, http.StatusOK, planCurrentResponse{
		L1: currentWork.L1,
		L2: currentWork.L2,
	})
	return nil
}

func planETag(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("\"%x\"", sum)
}

func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
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

func (h *RestHandler) listWorkflowSummaries(ctx context.Context) []workflowSummary {
	temporalClient := h.Manager.TemporalClient()
	if temporalClient == nil {
		return []workflowSummary{}
	}

	infos := h.Manager.List()
	summaries := make([]workflowSummary, 0, len(infos))
	for _, info := range infos {
		session, ok := h.Manager.Get(info.ID)
		if !ok {
			continue
		}
		workflowID, workflowRunID, ok := session.WorkflowIdentifiers()
		if !ok {
			continue
		}

		summary := workflowSummary{
			SessionID:     info.ID,
			WorkflowID:    workflowID,
			WorkflowRunID: workflowRunID,
			Title:         info.Title,
			Role:          info.Role,
		}

		state, err := queryWorkflowState(ctx, temporalClient, workflowID, workflowRunID)
		if err != nil {
			var notFound *serviceerror.NotFound
			if errors.As(err, &notFound) {
				summary.Status = workflows.SessionStatusStopped
			} else {
				summary.Status = workflowStatusUnknown
			}
			summary.StartTime = info.CreatedAt
			if h.Logger != nil {
				h.Logger.Warn("workflow status query failed", map[string]string{
					"workflow_id": workflowID,
					"run_id":      workflowRunID,
					"error":       err.Error(),
				})
			}
		} else {
			summary.AgentName = state.AgentID
			summary.CurrentL1 = state.CurrentL1
			summary.CurrentL2 = state.CurrentL2
			summary.Status = state.Status
			if summary.Status == "" {
				summary.Status = workflowStatusUnknown
			}
			summary.StartTime = state.StartTime
			if summary.StartTime.IsZero() {
				summary.StartTime = info.CreatedAt
			}
			if len(state.BellEvents) > 0 {
				summary.BellEvents = make([]workflowBellEvent, 0, len(state.BellEvents))
				for _, event := range state.BellEvents {
					summary.BellEvents = append(summary.BellEvents, workflowBellEvent{
						Timestamp: event.Timestamp,
						Context:   event.Context,
					})
				}
			}
			if len(state.TaskEvents) > 0 {
				summary.TaskEvents = make([]workflowTaskEvent, 0, len(state.TaskEvents))
				for _, event := range state.TaskEvents {
					summary.TaskEvents = append(summary.TaskEvents, workflowTaskEvent{
						Timestamp: event.Timestamp,
						L1:        event.L1,
						L2:        event.L2,
					})
				}
			}
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].StartTime.Equal(summaries[j].StartTime) {
			return summaries[i].SessionID < summaries[j].SessionID
		}
		return summaries[i].StartTime.After(summaries[j].StartTime)
	})

	return summaries
}

func queryWorkflowState(ctx context.Context, temporalClient temporal.WorkflowClient, workflowID, workflowRunID string) (workflows.SessionWorkflowState, error) {
	var state workflows.SessionWorkflowState
	if temporalClient == nil {
		return state, errors.New("temporal client unavailable")
	}
	if workflowID == "" {
		return state, errors.New("workflow id required")
	}

	queryContext, cancel := context.WithTimeout(ctx, workflowQueryTimeout)
	defer cancel()

	encodedValue, err := temporalClient.QueryWorkflow(queryContext, workflowID, workflowRunID, workflows.StatusQueryName)
	if err != nil {
		return state, err
	}
	if encodedValue == nil || !encodedValue.HasValue() {
		return state, errors.New("workflow status unavailable")
	}
	if err := encodedValue.Get(&state); err != nil {
		return state, err
	}
	return state, nil
}

func fetchWorkflowHistoryEntries(ctx context.Context, temporalClient temporal.WorkflowClient, workflowID, workflowRunID string, logger *logging.Logger) ([]workflowHistoryEntry, error) {
	if temporalClient == nil {
		return nil, errors.New("temporal client unavailable")
	}
	if workflowID == "" {
		return nil, errors.New("workflow id required")
	}

	historyContext, cancel := context.WithTimeout(ctx, workflowHistoryTimeout)
	defer cancel()

	iterator := temporalClient.GetWorkflowHistory(historyContext, workflowID, workflowRunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	if iterator == nil {
		return nil, errors.New("workflow history unavailable")
	}

	entries := []workflowHistoryEntry{}
	dataConverter := converter.GetDefaultDataConverter()

	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}

		eventTime := time.Time{}
		if timestamp := event.GetEventTime(); timestamp != nil {
			eventTime = timestamp.AsTime()
		}

		if event.GetEventType() != enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED {
			continue
		}

		attributes := event.GetWorkflowExecutionSignaledEventAttributes()
		if attributes == nil {
			continue
		}

		signalName := attributes.GetSignalName()
		entry := workflowHistoryEntry{
			EventID:    event.GetEventId(),
			Timestamp:  eventTime,
			SignalName: signalName,
		}

		switch signalName {
		case workflows.UpdateTaskSignalName:
			var payload workflows.UpdateTaskSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "task_update"
				entry.L1 = payload.L1
				entry.L2 = payload.L2
			} else {
				entry.Type = "signal"
			}
		case workflows.BellSignalName:
			var payload workflows.BellSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "bell"
				entry.Context = payload.Context
				if !payload.Timestamp.IsZero() {
					entry.Timestamp = payload.Timestamp
				}
			} else {
				entry.Type = "signal"
			}
		case workflows.ResumeSignalName:
			var payload workflows.ResumeSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "resume"
				entry.Action = payload.Action
			} else {
				entry.Type = "signal"
			}
		case workflows.TerminateSignalName:
			var payload workflows.TerminateSignal
			if decodeSignalPayload(dataConverter, attributes.GetInput(), &payload, logger, signalName) {
				entry.Type = "terminate"
				entry.Reason = payload.Reason
			} else {
				entry.Type = "signal"
			}
		default:
			entry.Type = "signal"
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func decodeSignalPayload(dataConverter converter.DataConverter, payloads *commonpb.Payloads, destination interface{}, logger *logging.Logger, signalName string) bool {
	if payloads == nil {
		return false
	}
	if dataConverter == nil {
		dataConverter = converter.GetDefaultDataConverter()
	}
	if err := dataConverter.FromPayloads(payloads, destination); err != nil {
		if logger != nil {
			logger.Warn("failed to decode workflow signal", map[string]string{
				"signal": signalName,
				"error":  err.Error(),
			})
		}
		return false
	}
	return true
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
				"agent_id":   request.Agent,
				"agent_name": agentProfile.Name,
				"hash":       agentProfile.ConfigHash,
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
			"terminal_id":   id,
			"context_lines": strconv.Itoa(len(contextLines)),
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
			"terminal_id": id,
			"workflow_id": workflowID,
			"run_id":      workflowRunID,
			"action":      action,
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
	if strings.HasSuffix(trimmed, "/workflow/history") {
		id := strings.TrimSuffix(trimmed, "/workflow/history")
		return id, terminalPathWorkflowHistory
	}
	if strings.HasSuffix(trimmed, "/history") {
		id := strings.TrimSuffix(trimmed, "/history")
		return id, terminalPathHistory
	}
	if strings.HasSuffix(trimmed, "/input-history") {
		id := strings.TrimSuffix(trimmed, "/input-history")
		return id, terminalPathInputHistory
	}
	if strings.HasSuffix(trimmed, "/bell") {
		id := strings.TrimSuffix(trimmed, "/bell")
		return id, terminalPathBell
	}
	if strings.HasSuffix(trimmed, "/workflow/resume") {
		id := strings.TrimSuffix(trimmed, "/workflow/resume")
		return id, terminalPathWorkflowResume
	}
	return trimmed, terminalPathTerminal
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

func loadGitInfo() (string, string) {
	workDir, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	gitDir := resolveGitDir(workDir)
	if gitDir == "" {
		return "", ""
	}
	origin := readGitOrigin(filepath.Join(gitDir, "config"))
	branch := readGitBranch(filepath.Join(gitDir, "HEAD"))
	return origin, branch
}

func resolveGitDir(workDir string) string {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	if !info.Mode().IsRegular() {
		return ""
	}
	contents, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	return gitDir
}

func readGitOrigin(configPath string) string {
	file, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != `remote "origin"` {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "url" {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func readGitBranch(headPath string) string {
	contents, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(contents))
	if line == "" {
		return ""
	}
	const prefix = "ref: "
	if strings.HasPrefix(line, prefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	short := line
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("detached@%s", short)
}
