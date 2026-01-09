package workflows

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

const (
	SessionStatusRunning = "running"
	SessionStatusPaused  = "paused"
	SessionStatusStopped = "stopped"

	SessionTaskQueueName = "gestalt-session"

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

func SessionWorkflow(workflowContext workflow.Context, request SessionWorkflowRequest) (SessionWorkflowResult, error) {
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
		return SessionWorkflowResult{}, queryError
	}

	updateTaskChannel := workflow.GetSignalChannel(workflowContext, UpdateTaskSignalName)
	bellChannel := workflow.GetSignalChannel(workflowContext, BellSignalName)
	resumeChannel := workflow.GetSignalChannel(workflowContext, ResumeSignalName)
	terminateChannel := workflow.GetSignalChannel(workflowContext, TerminateSignalName)

	eventCount := 0
	logger := workflow.GetLogger(workflowContext)
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
		eventCount++
	})

	selector.AddReceive(bellChannel, func(channel workflow.ReceiveChannel, more bool) {
		var signal BellSignal
		channel.Receive(workflowContext, &signal)
		timestamp := signal.Timestamp
		if timestamp.IsZero() {
			timestamp = workflow.Now(workflowContext)
		}
		state.BellEvents = append(state.BellEvents, BellEvent{
			Timestamp: timestamp,
			Context:   signal.Context,
		})
		state.Status = SessionStatusPaused
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

	return SessionWorkflowResult{
		SessionID:   state.SessionID,
		EndTime:     workflow.Now(workflowContext),
		FinalStatus: state.Status,
		EventCount:  eventCount,
	}, nil
}
