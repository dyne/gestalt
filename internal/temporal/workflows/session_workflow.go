package workflows

import (
	"time"

	"gestalt/internal/metrics"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	SessionStatusRunning = "running"
	SessionStatusPaused  = "paused"
	SessionStatusStopped = "stopped"

	SessionTaskQueueName = "gestalt-session"

	SpawnTerminalActivityName = "SpawnTerminalActivity"
	UpdateTaskActivityName    = "UpdateTaskActivity"
	RecordBellActivityName    = "RecordBellActivity"
	GetOutputActivityName     = "GetOutputActivity"

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

	StatusQueryName = "session.status"

	ResumeActionContinue = "continue"
	ResumeActionAbort    = "abort"
	ResumeActionHandoff  = "handoff"
)

type SessionWorkflowRequest struct {
	SessionID string
	AgentID   string
	L1Task    string
	L2Task    string
	Shell     string
	StartTime time.Time
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

func SessionWorkflow(workflowContext workflow.Context, request SessionWorkflowRequest) (result SessionWorkflowResult, err error) {
	metrics.Default.IncWorkflowStarted()
	defer func() {
		if err != nil {
			metrics.Default.IncWorkflowFailed()
		} else {
			metrics.Default.IncWorkflowCompleted()
		}
	}()

	spawnContext := workflow.WithActivityOptions(workflowContext, workflow.ActivityOptions{
		StartToCloseTimeout: SpawnTerminalTimeout,
		HeartbeatTimeout:    DefaultActivityHeartbeat,
		RetryPolicy:         defaultActivityRetryPolicy(),
	})
	if activityErr := workflow.ExecuteActivity(spawnContext, SpawnTerminalActivityName, request.SessionID, request.Shell).Get(spawnContext, nil); activityErr != nil {
		err = activityErr
		return SessionWorkflowResult{}, activityErr
	}

	state := SessionWorkflowState{
		SessionID: request.SessionID,
		AgentID:   request.AgentID,
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

	queryError := workflow.SetQueryHandler(workflowContext, StatusQueryName, func() (SessionWorkflowState, error) {
		return state, nil
	})
	if queryError != nil {
		err = queryError
		return SessionWorkflowResult{}, queryError
	}

	updateTaskChannel := workflow.GetSignalChannel(workflowContext, UpdateTaskSignalName)
	bellChannel := workflow.GetSignalChannel(workflowContext, BellSignalName)
	resumeChannel := workflow.GetSignalChannel(workflowContext, ResumeSignalName)
	terminateChannel := workflow.GetSignalChannel(workflowContext, TerminateSignalName)

	eventCount := 0
	logger := workflow.GetLogger(workflowContext)
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
		eventCount++
	})

	selector.AddReceive(resumeChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal ResumeSignal
		channel.Receive(workflowContext, &signal)
		switch signal.Action {
		case ResumeActionAbort:
			state.Status = SessionStatusStopped
		case ResumeActionContinue, ResumeActionHandoff, "":
			state.Status = SessionStatusRunning
		default:
			logger.Warn("unknown resume action", "action", signal.Action)
			state.Status = SessionStatusRunning
		}
		eventCount++
	})

	selector.AddReceive(terminateChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal TerminateSignal
		channel.Receive(workflowContext, &signal)
		state.Status = SessionStatusStopped
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
