package api

import (
	"sync"
	"time"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
	"gestalt/internal/terminal"
)

type RestHandler struct {
	Manager        *terminal.Manager
	Logger         *logging.Logger
	MetricsSummary *otel.APISummaryStore
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
	ID     string   `json:"id"`
	Lines  []string `json:"lines"`
	Cursor *int64   `json:"cursor,omitempty"`
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

type planHeading struct {
	Level    int           `json:"level"`
	Keyword  string        `json:"keyword"`
	Priority string        `json:"priority"`
	Text     string        `json:"text"`
	Body     string        `json:"body"`
	Children []planHeading `json:"children"`
}

type planDocument struct {
	Filename  string        `json:"filename"`
	Title     string        `json:"title"`
	Subtitle  string        `json:"subtitle"`
	Date      string        `json:"date"`
	Keywords  string        `json:"keywords"`
	Headings  []planHeading `json:"headings"`
	L1Count   int           `json:"l1_count"`
	L2Count   int           `json:"l2_count"`
	PriorityA int           `json:"priority_a"`
	PriorityB int           `json:"priority_b"`
	PriorityC int           `json:"priority_c"`
}

type plansListResponse struct {
	Plans []planDocument `json:"plans"`
}

type errorResponse struct {
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
	Code       string `json:"code,omitempty"`
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
