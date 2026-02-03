package workflows

import (
	"encoding/json"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/metrics"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	SessionStatusRunning = "running"
	SessionStatusPaused  = "paused"
	SessionStatusStopped = "stopped"

	SessionTaskQueueName = "gestalt-session"

	SpawnTerminalActivityName     = "SpawnTerminalActivity"
	UpdateTaskActivityName        = "UpdateTaskActivity"
	RecordBellActivityName        = "RecordBellActivity"
	GetOutputActivityName         = "GetOutputActivity"
	EmitWorkflowEventActivityName = "EmitWorkflowEventActivity"

	DefaultWorkflowExecutionTimeout = 24 * time.Hour
	DefaultWorkflowRunTimeout       = 24 * time.Hour
	DefaultWorkflowTaskTimeout      = 10 * time.Second

	DefaultActivityTimeout       = 10 * time.Second
	SpawnTerminalTimeout         = 30 * time.Second
	ReadOutputTimeout            = 5 * time.Second
	DefaultActivityHeartbeat     = 10 * time.Second
	DefaultActivityRetryAttempts = 5

	UpdateTaskSignalName = "session.update_task"
	BellSignalName       = "session.bell"
	ResumeSignalName     = "session.resume"
	TerminateSignalName  = "session.terminate"
	NotifySignalName     = "session.notify"

	StatusQueryName = "session.status"

	ResumeActionContinue = "continue"
	ResumeActionAbort    = "abort"
	ResumeActionHandoff  = "handoff"
)

type SessionWorkflowRequest struct {
	SessionID             string
	AgentID               string
	AgentName             string
	L1Task                string
	L2Task                string
	Shell                 string
	AgentConfig           string
	ConfigHash            string
	StartTime             time.Time
	CollectorStartTime    time.Time
	CollectorGRPCEndpoint string
	CollectorHTTPEndpoint string
	CollectorConfigPath   string
	CollectorDataPath     string
}

type SessionWorkflowResult struct {
	SessionID   string
	EndTime     time.Time
	FinalStatus string
	EventCount  int
}

type SessionWorkflowState struct {
	SessionID  string
	AgentID    string
	AgentName  string
	CurrentL1  string
	CurrentL2  string
	Status     string
	StartTime  time.Time
	BellEvents []BellEvent
	TaskEvents []TaskEvent
}

type BellEvent struct {
	Timestamp time.Time
	Context   string
}

type TaskEvent struct {
	Timestamp time.Time
	L1        string
	L2        string
}

type UpdateTaskSignal struct {
	L1 string
	L2 string
}

type BellSignal struct {
	Timestamp time.Time
	Context   string
}

type ResumeSignal struct {
	Action string
}

type TerminateSignal struct {
	Reason string
}

type NotifySignal struct {
	Timestamp  time.Time
	SessionID  string
	AgentID    string
	AgentName  string
	EventType  string
	Payload    json.RawMessage
	Raw        string
	EventID    string
}

func SessionWorkflow(workflowContext workflow.Context, request SessionWorkflowRequest) (result SessionWorkflowResult, err error) {
	metrics.Default.IncWorkflowStarted()
	defer func() {
		if err != nil {
			metrics.Default.IncWorkflowFailed()
		} else {
			metrics.Default.IncWorkflowCompleted()
		}
	}()

	logger := workflow.GetLogger(workflowContext)
	workflowInfo := workflow.GetInfo(workflowContext)
	workflowID := workflowInfo.WorkflowExecution.ID
	eventContext := workflow.WithActivityOptions(workflowContext, workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    DefaultActivityHeartbeat,
		RetryPolicy:         defaultActivityRetryPolicy(),
	})
	emitWorkflowEvent := func(eventType string, occurredAt time.Time, context map[string]any) {
		if eventType == "" {
			return
		}
		timestamp := occurredAt
		if timestamp.IsZero() {
			timestamp = workflow.Now(workflowContext)
		}
		payload := event.WorkflowEvent{
			EventType:  eventType,
			WorkflowID: workflowID,
			SessionID:  request.SessionID,
			OccurredAt: timestamp,
			Context:    context,
		}
		if activityErr := workflow.ExecuteActivity(eventContext, EmitWorkflowEventActivityName, payload).Get(eventContext, nil); activityErr != nil {
			logger.Warn("workflow event activity failed", "error", activityErr, "event_type", eventType)
		}
	}

	spawnContext := workflow.WithActivityOptions(workflowContext, workflow.ActivityOptions{
		StartToCloseTimeout: SpawnTerminalTimeout,
		HeartbeatTimeout:    DefaultActivityHeartbeat,
		RetryPolicy:         defaultActivityRetryPolicy(),
	})
	if activityErr := workflow.ExecuteActivity(spawnContext, SpawnTerminalActivityName, request.SessionID, request.AgentID, request.Shell).Get(spawnContext, nil); activityErr != nil {
		emitWorkflowEvent("workflow_error", workflow.Now(workflowContext), map[string]any{
			"error": activityErr.Error(),
			"stage": "spawn",
		})
		err = activityErr
		return SessionWorkflowResult{}, activityErr
	}

	state := SessionWorkflowState{
		SessionID: request.SessionID,
		AgentID:   request.AgentID,
		AgentName: request.AgentName,
		CurrentL1: request.L1Task,
		CurrentL2: request.L2Task,
		Status:    SessionStatusRunning,
		StartTime: request.StartTime,
	}
	if state.StartTime.IsZero() {
		state.StartTime = workflow.Now(workflowContext)
	}
	if state.CurrentL1 != "" || state.CurrentL2 != "" {
		state.TaskEvents = append(state.TaskEvents, TaskEvent{
			Timestamp: state.StartTime,
			L1:        state.CurrentL1,
			L2:        state.CurrentL2,
		})
	}
	var startedContext map[string]any
	if request.AgentID != "" || request.AgentName != "" {
		startedContext = map[string]any{}
		if request.AgentID != "" {
			startedContext["agent_id"] = request.AgentID
		}
		if request.AgentName != "" {
			startedContext["agent_name"] = request.AgentName
		}
	}
	if !request.CollectorStartTime.IsZero() {
		if startedContext == nil {
			startedContext = map[string]any{}
		}
		startedContext["otel_started_at"] = request.CollectorStartTime
	}
	if request.CollectorGRPCEndpoint != "" {
		if startedContext == nil {
			startedContext = map[string]any{}
		}
		startedContext["otel_grpc_endpoint"] = request.CollectorGRPCEndpoint
	}
	if request.CollectorHTTPEndpoint != "" {
		if startedContext == nil {
			startedContext = map[string]any{}
		}
		startedContext["otel_http_endpoint"] = request.CollectorHTTPEndpoint
	}
	if request.CollectorConfigPath != "" {
		if startedContext == nil {
			startedContext = map[string]any{}
		}
		startedContext["otel_config_path"] = request.CollectorConfigPath
	}
	if request.CollectorDataPath != "" {
		if startedContext == nil {
			startedContext = map[string]any{}
		}
		startedContext["otel_data_path"] = request.CollectorDataPath
	}
	emitWorkflowEvent("workflow_started", state.StartTime, startedContext)

	queryError := workflow.SetQueryHandler(workflowContext, StatusQueryName, func() (SessionWorkflowState, error) {
		return state, nil
	})
	if queryError != nil {
		emitWorkflowEvent("workflow_error", workflow.Now(workflowContext), map[string]any{
			"error": queryError.Error(),
			"stage": "query_handler",
		})
		err = queryError
		return SessionWorkflowResult{}, queryError
	}

	updateTaskChannel := workflow.GetSignalChannel(workflowContext, UpdateTaskSignalName)
	bellChannel := workflow.GetSignalChannel(workflowContext, BellSignalName)
	resumeChannel := workflow.GetSignalChannel(workflowContext, ResumeSignalName)
	terminateChannel := workflow.GetSignalChannel(workflowContext, TerminateSignalName)
	notifyChannel := workflow.GetSignalChannel(workflowContext, NotifySignalName)

	eventCount := 0
	var completionContext map[string]any
	activityContext := workflow.WithActivityOptions(workflowContext, workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    DefaultActivityHeartbeat,
		RetryPolicy:         defaultActivityRetryPolicy(),
	})
	readContext := workflow.WithActivityOptions(workflowContext, workflow.ActivityOptions{
		StartToCloseTimeout: ReadOutputTimeout,
		RetryPolicy:         defaultActivityRetryPolicy(),
	})
	selector := workflow.NewSelector(workflowContext)

	selector.AddReceive(updateTaskChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal UpdateTaskSignal
		channel.Receive(workflowContext, &signal)
		state.CurrentL1 = signal.L1
		state.CurrentL2 = signal.L2
		state.TaskEvents = append(state.TaskEvents, TaskEvent{
			Timestamp: workflow.Now(workflowContext),
			L1:        state.CurrentL1,
			L2:        state.CurrentL2,
		})
		if activityErr := workflow.ExecuteActivity(activityContext, UpdateTaskActivityName, request.SessionID, signal.L1, signal.L2).Get(activityContext, nil); activityErr != nil {
			logger.Warn("update task activity failed", "error", activityErr)
		}
		eventCount++
	})

	selector.AddReceive(bellChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal BellSignal
		channel.Receive(workflowContext, &signal)
		timestamp := signal.Timestamp
		if timestamp.IsZero() {
			timestamp = workflow.Now(workflowContext)
		}
		contextText := signal.Context
		if contextText == "" {
			var output string
			if activityErr := workflow.ExecuteActivity(readContext, GetOutputActivityName, request.SessionID).Get(readContext, &output); activityErr != nil {
				logger.Warn("get output activity failed", "error", activityErr)
			} else {
				contextText = output
			}
		}
		state.BellEvents = append(state.BellEvents, BellEvent{
			Timestamp: timestamp,
			Context:   contextText,
		})
		if activityErr := workflow.ExecuteActivity(activityContext, RecordBellActivityName, request.SessionID, timestamp, contextText).Get(activityContext, nil); activityErr != nil {
			logger.Warn("record bell activity failed", "error", activityErr)
		}
		state.Status = SessionStatusPaused
		metrics.Default.IncWorkflowPaused()
		var pauseContext map[string]any
		if contextText != "" {
			pauseContext = map[string]any{
				"bell_context": contextText,
			}
		}
		emitWorkflowEvent("workflow_paused", timestamp, pauseContext)
		eventCount++
	})

	selector.AddReceive(notifyChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal NotifySignal
		channel.Receive(workflowContext, &signal)
		timestamp := signal.Timestamp
		if timestamp.IsZero() {
			timestamp = workflow.Now(workflowContext)
		}
		var notifyContext map[string]any
		if signal.EventType != "" {
			notifyContext = map[string]any{
				"event_type": signal.EventType,
			}
		}
		if signal.EventID != "" {
			if notifyContext == nil {
				notifyContext = map[string]any{}
			}
			notifyContext["event_id"] = signal.EventID
		}
		emitWorkflowEvent("notify_event", timestamp, notifyContext)
		eventCount++
	})

	selector.AddReceive(resumeChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal ResumeSignal
		channel.Receive(workflowContext, &signal)
		switch signal.Action {
		case ResumeActionAbort:
			state.Status = SessionStatusStopped
			completionContext = map[string]any{
				"action": signal.Action,
			}
		case ResumeActionContinue, ResumeActionHandoff, "":
			state.Status = SessionStatusRunning
		default:
			logger.Warn("unknown resume action", "action", signal.Action)
			state.Status = SessionStatusRunning
		}
		if state.Status == SessionStatusRunning {
			var resumeContext map[string]any
			if signal.Action != "" {
				resumeContext = map[string]any{
					"action": signal.Action,
				}
			}
			emitWorkflowEvent("workflow_resumed", workflow.Now(workflowContext), resumeContext)
		}
		eventCount++
	})

	selector.AddReceive(terminateChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal TerminateSignal
		channel.Receive(workflowContext, &signal)
		state.Status = SessionStatusStopped
		if signal.Reason != "" {
			completionContext = map[string]any{
				"reason": signal.Reason,
			}
		}
		eventCount++
	})

	for state.Status != SessionStatusStopped {
		selector.Select(workflowContext)
	}

	result = SessionWorkflowResult{
		SessionID:   state.SessionID,
		EndTime:     workflow.Now(workflowContext),
		FinalStatus: state.Status,
		EventCount:  eventCount,
	}
	emitWorkflowEvent("workflow_completed", result.EndTime, completionContext)
	return result, nil
}

func defaultActivityRetryPolicy() *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:    time.Second,
		BackoffCoefficient: 2.0,
		MaximumInterval:    30 * time.Second,
		MaximumAttempts:    DefaultActivityRetryAttempts,
	}
}
