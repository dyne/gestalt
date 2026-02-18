package api

import (
	"encoding/json"
	"sync"
	"time"

	"gestalt/internal/flow"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/otel"
	"gestalt/internal/runner/launchspec"
	"gestalt/internal/terminal"
)

type RestHandler struct {
	Manager                *terminal.Manager
	FlowService            *flow.Service
	NotificationSink       notify.Sink
	Logger                 *logging.Logger
	MetricsSummary         *otel.APISummaryStore
	GitOrigin              string
	GitBranch              string
	SessionScrollbackLines int
	SessionFontFamily      string
	SessionFontSize        string
	SessionInputFontFamily string
	SessionInputFontSize   string
	gitMutex               sync.RWMutex
}

type terminalSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
	LLMType     string    `json:"llm_type"`
	LLMModel    string    `json:"llm_model"`
	Interface   string    `json:"interface"`
	Runner      string    `json:"runner,omitempty"`
	Command     string    `json:"command,omitempty"`
	Skills      []string  `json:"skills"`
	PromptFiles []string  `json:"prompt_files"`
	GUIModules  []string  `json:"gui_modules"`
}

type terminalCreateResponse struct {
	terminalSummary
	Launch *launchspec.LaunchSpec `json:"launch,omitempty"`
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
	SessionCount              int       `json:"session_count"`
	ServerTime                time.Time `json:"server_time"`
	SessionPersist            bool      `json:"session_persist"`
	SessionScrollbackLines    int       `json:"session_scrollback_lines"`
	SessionFontFamily         string    `json:"session_font_family"`
	SessionFontSize           string    `json:"session_font_size"`
	SessionInputFontFamily    string    `json:"session_input_font_family"`
	SessionInputFontSize      string    `json:"session_input_font_size"`
	AgentsSessionID           string    `json:"agents_session_id,omitempty"`
	AgentsTmuxSession         string    `json:"agents_tmux_session,omitempty"`
	WorkingDir                string    `json:"working_dir"`
	GitOrigin                 string    `json:"git_origin"`
	GitBranch                 string    `json:"git_branch"`
	Version                   string    `json:"version"`
	Major                     int       `json:"major"`
	Minor                     int       `json:"minor"`
	Patch                     int       `json:"patch"`
	Built                     string    `json:"built"`
	GitCommit                 string    `json:"git_commit,omitempty"`
	OTelCollectorRunning      bool      `json:"otel_collector_running"`
	OTelCollectorPID          int       `json:"otel_collector_pid"`
	OTelCollectorHTTPEndpoint string    `json:"otel_collector_http_endpoint"`
	OTelCollectorLastExit     string    `json:"otel_collector_last_exit,omitempty"`
	OTelCollectorRestartCount int       `json:"otel_collector_restart_count"`
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
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
	Code      string `json:"code,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type agentSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LLMType   string `json:"llm_type"`
	LLMModel  string `json:"llm_model"`
	Interface string `json:"interface"`
	SessionID string `json:"session_id"`
	Running   bool   `json:"running"`
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
	Title      string   `json:"title"`
	Role       string   `json:"role"`
	Agent      string   `json:"agent"`
	Runner     string   `json:"runner,omitempty"`
	GUIModules []string `json:"gui_modules,omitempty"`
}

type terminalProgressResponse struct {
	HasProgress bool       `json:"has_progress"`
	PlanFile    string     `json:"plan_file,omitempty"`
	L1          string     `json:"l1,omitempty"`
	L2          string     `json:"l2,omitempty"`
	TaskLevel   int        `json:"task_level,omitempty"`
	TaskState   string     `json:"task_state,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type notifyRequest struct {
	SessionID  string          `json:"session_id"`
	EventType  string          `json:"-"`
	OccurredAt *time.Time      `json:"occurred_at,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Raw        string          `json:"raw,omitempty"`
	EventID    string          `json:"event_id,omitempty"`
}

type terminalPathAction int

const (
	terminalPathTerminal terminalPathAction = iota
	terminalPathOutput
	terminalPathHistory
	terminalPathInput
	terminalPathActivate
	terminalPathInputHistory
	terminalPathBell
	terminalPathNotify
	terminalPathProgress
	terminalPathWorkflowResume
	terminalPathWorkflowHistory
)
