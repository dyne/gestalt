package workflows

import (
	"context"
	"testing"
	"time"

	"gestalt/internal/event"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestSessionWorkflowSignals(testingContext *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()
	workflowEnvironment.RegisterWorkflow(SessionWorkflow)
	var emittedEvents []event.WorkflowEvent
	registerSessionActivities(workflowEnvironment, func(payload event.WorkflowEvent) {
		emittedEvents = append(emittedEvents, payload)
	})

	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	bellTime := time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC)
	notifyTime := startTime.Add(90 * time.Second)

	var statusAfterUpdate SessionWorkflowState
	var statusAfterBell SessionWorkflowState
	var updateQueryError error
	var bellQueryError error

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(UpdateTaskSignalName, UpdateTaskSignal{
			L1: "Updated L1",
			L2: "Updated L2",
		})
	}, time.Minute)

	workflowEnvironment.RegisterDelayedCallback(func() {
		queryResult, queryError := workflowEnvironment.QueryWorkflow(StatusQueryName)
		if queryError != nil {
			updateQueryError = queryError
			return
		}
		updateQueryError = queryResult.Get(&statusAfterUpdate)
	}, time.Minute+time.Second)

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(NotifySignalName, NotifySignal{
			Timestamp: notifyTime,
			EventType: "agent-turn-complete",
			Source:    "codex-notify",
		})
	}, time.Minute+30*time.Second)

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(BellSignalName, BellSignal{
			Timestamp: bellTime,
			Context:   "bell context",
		})
	}, 2*time.Minute)

	workflowEnvironment.RegisterDelayedCallback(func() {
		queryResult, queryError := workflowEnvironment.QueryWorkflow(StatusQueryName)
		if queryError != nil {
			bellQueryError = queryError
			return
		}
		bellQueryError = queryResult.Get(&statusAfterBell)
	}, 2*time.Minute+time.Second)

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(ResumeSignalName, ResumeSignal{
			Action: ResumeActionContinue,
		})
	}, 3*time.Minute)

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(TerminateSignalName, TerminateSignal{
			Reason: "complete",
		})
	}, 4*time.Minute)

	workflowEnvironment.ExecuteWorkflow(SessionWorkflow, SessionWorkflowRequest{
		SessionID: "session-1",
		AgentID:   "agent-1",
		AgentName: "Agent One",
		L1Task:    "Initial L1",
		L2Task:    "Initial L2",
		Shell:     "/bin/bash",
		StartTime: startTime,
	})

	if !workflowEnvironment.IsWorkflowCompleted() {
		testingContext.Fatal("workflow did not complete")
	}
	if workflowEnvironment.GetWorkflowError() != nil {
		testingContext.Fatalf("workflow error: %v", workflowEnvironment.GetWorkflowError())
	}

	if updateQueryError != nil {
		testingContext.Fatalf("update query failed: %v", updateQueryError)
	}
	if bellQueryError != nil {
		testingContext.Fatalf("bell query failed: %v", bellQueryError)
	}
	if statusAfterUpdate.CurrentL1 != "Updated L1" || statusAfterUpdate.CurrentL2 != "Updated L2" {
		testingContext.Fatalf("task update not recorded: %#v", statusAfterUpdate)
	}
	if statusAfterUpdate.AgentID != "agent-1" {
		testingContext.Fatalf("unexpected agent id: %#v", statusAfterUpdate.AgentID)
	}
	if statusAfterUpdate.AgentName != "Agent One" {
		testingContext.Fatalf("unexpected agent name: %#v", statusAfterUpdate.AgentName)
	}
	if len(statusAfterUpdate.TaskEvents) != 2 {
		testingContext.Fatalf("expected 2 task events, got %d", len(statusAfterUpdate.TaskEvents))
	}
	if statusAfterUpdate.TaskEvents[0].L1 != "Initial L1" || statusAfterUpdate.TaskEvents[0].L2 != "Initial L2" {
		testingContext.Fatalf("unexpected initial task event: %#v", statusAfterUpdate.TaskEvents[0])
	}
	if !statusAfterUpdate.TaskEvents[0].Timestamp.Equal(startTime) {
		testingContext.Fatalf("unexpected initial task timestamp: %v", statusAfterUpdate.TaskEvents[0].Timestamp)
	}
	if statusAfterUpdate.TaskEvents[1].L1 != "Updated L1" || statusAfterUpdate.TaskEvents[1].L2 != "Updated L2" {
		testingContext.Fatalf("unexpected updated task event: %#v", statusAfterUpdate.TaskEvents[1])
	}
	if statusAfterUpdate.Status != SessionStatusRunning {
		testingContext.Fatalf("expected running after task update, got %s", statusAfterUpdate.Status)
	}
	if statusAfterBell.Status != SessionStatusPaused {
		testingContext.Fatalf("expected paused after bell, got %s", statusAfterBell.Status)
	}
	if len(statusAfterBell.BellEvents) != 1 {
		testingContext.Fatalf("expected 1 bell event, got %d", len(statusAfterBell.BellEvents))
	}
	if statusAfterBell.BellEvents[0].Context != "bell context" {
		testingContext.Fatalf("unexpected bell context: %#v", statusAfterBell.BellEvents[0])
	}

	var workflowResult SessionWorkflowResult
	resultError := workflowEnvironment.GetWorkflowResult(&workflowResult)
	if resultError != nil {
		testingContext.Fatalf("result error: %v", resultError)
	}
	if workflowResult.FinalStatus != SessionStatusStopped {
		testingContext.Fatalf("expected stopped result, got %s", workflowResult.FinalStatus)
	}
	if workflowResult.EventCount != 5 {
		testingContext.Fatalf("expected 5 events, got %d", workflowResult.EventCount)
	}

	expectedEvents := []string{"workflow_started", "notify_event", "workflow_paused", "workflow_resumed", "workflow_completed"}
	if len(emittedEvents) != len(expectedEvents) {
		testingContext.Fatalf("expected %d workflow events, got %d", len(expectedEvents), len(emittedEvents))
	}
	for index, eventType := range expectedEvents {
		if emittedEvents[index].Type() != eventType {
			testingContext.Fatalf("expected event %q at index %d, got %q", eventType, index, emittedEvents[index].Type())
		}
		if emittedEvents[index].WorkflowID == "" {
			testingContext.Fatalf("expected workflow id for %q event", emittedEvents[index].Type())
		}
		if emittedEvents[index].SessionID != "session-1" {
			testingContext.Fatalf("expected session id session-1, got %q", emittedEvents[index].SessionID)
		}
	}
	if !emittedEvents[0].Timestamp().Equal(startTime) {
		testingContext.Fatalf("expected start timestamp %v, got %v", startTime, emittedEvents[0].Timestamp())
	}
	notifyType, ok := emittedEvents[1].Context["event_type"].(string)
	if !ok || notifyType != "agent-turn-complete" {
		testingContext.Fatalf("unexpected notify context: %#v", emittedEvents[1].Context)
	}
	if !emittedEvents[1].Timestamp().Equal(notifyTime) {
		testingContext.Fatalf("expected notify timestamp %v, got %v", notifyTime, emittedEvents[1].Timestamp())
	}
	pausedContext, ok := emittedEvents[2].Context["bell_context"].(string)
	if !ok || pausedContext != "bell context" {
		testingContext.Fatalf("unexpected paused context: %#v", emittedEvents[2].Context)
	}
	if !emittedEvents[2].Timestamp().Equal(bellTime) {
		testingContext.Fatalf("expected bell timestamp %v, got %v", bellTime, emittedEvents[2].Timestamp())
	}
	resumeAction, ok := emittedEvents[3].Context["action"].(string)
	if !ok || resumeAction != ResumeActionContinue {
		testingContext.Fatalf("unexpected resume context: %#v", emittedEvents[3].Context)
	}
	completionReason, ok := emittedEvents[4].Context["reason"].(string)
	if !ok || completionReason != "complete" {
		testingContext.Fatalf("unexpected completion context: %#v", emittedEvents[4].Context)
	}
}

func TestSessionWorkflowAbortAction(testingContext *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()
	workflowEnvironment.RegisterWorkflow(SessionWorkflow)
	var emittedEvents []event.WorkflowEvent
	registerSessionActivities(workflowEnvironment, func(payload event.WorkflowEvent) {
		emittedEvents = append(emittedEvents, payload)
	})

	var statusAfterBell SessionWorkflowState
	var bellQueryError error

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(BellSignalName, BellSignal{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
			Context:   "pause",
		})
	}, time.Minute)

	workflowEnvironment.RegisterDelayedCallback(func() {
		queryResult, queryError := workflowEnvironment.QueryWorkflow(StatusQueryName)
		if queryError != nil {
			bellQueryError = queryError
			return
		}
		bellQueryError = queryResult.Get(&statusAfterBell)
	}, time.Minute+time.Second)

	workflowEnvironment.RegisterDelayedCallback(func() {
		workflowEnvironment.SignalWorkflow(ResumeSignalName, ResumeSignal{
			Action: ResumeActionAbort,
		})
	}, 2*time.Minute)

	workflowEnvironment.ExecuteWorkflow(SessionWorkflow, SessionWorkflowRequest{
		SessionID: "session-2",
		AgentID:   "agent-2",
		L1Task:    "Abort L1",
		L2Task:    "Abort L2",
		Shell:     "/bin/bash",
	})

	if !workflowEnvironment.IsWorkflowCompleted() {
		testingContext.Fatal("workflow did not complete")
	}
	if workflowEnvironment.GetWorkflowError() != nil {
		testingContext.Fatalf("workflow error: %v", workflowEnvironment.GetWorkflowError())
	}
	if bellQueryError != nil {
		testingContext.Fatalf("bell query failed: %v", bellQueryError)
	}
	if statusAfterBell.Status != SessionStatusPaused {
		testingContext.Fatalf("expected paused after bell, got %s", statusAfterBell.Status)
	}

	var workflowResult SessionWorkflowResult
	resultError := workflowEnvironment.GetWorkflowResult(&workflowResult)
	if resultError != nil {
		testingContext.Fatalf("result error: %v", resultError)
	}
	if workflowResult.FinalStatus != SessionStatusStopped {
		testingContext.Fatalf("expected stopped result, got %s", workflowResult.FinalStatus)
	}
	if workflowResult.EventCount != 2 {
		testingContext.Fatalf("expected 2 events, got %d", workflowResult.EventCount)
	}

	expectedEvents := []string{"workflow_started", "workflow_paused", "workflow_completed"}
	if len(emittedEvents) != len(expectedEvents) {
		testingContext.Fatalf("expected %d workflow events, got %d", len(expectedEvents), len(emittedEvents))
	}
	for index, eventType := range expectedEvents {
		if emittedEvents[index].Type() != eventType {
			testingContext.Fatalf("expected event %q at index %d, got %q", eventType, index, emittedEvents[index].Type())
		}
	}
	completionAction, ok := emittedEvents[2].Context["action"].(string)
	if !ok || completionAction != ResumeActionAbort {
		testingContext.Fatalf("unexpected completion context: %#v", emittedEvents[2].Context)
	}
}

func registerSessionActivities(workflowEnvironment *testsuite.TestWorkflowEnvironment, emitEvent func(event.WorkflowEvent)) {
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, sessionID, shell string) error {
			return nil
		},
		activity.RegisterOptions{Name: SpawnTerminalActivityName},
	)
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, sessionID, l1Task, l2Task string) error {
			return nil
		},
		activity.RegisterOptions{Name: UpdateTaskActivityName},
	)
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, sessionID string, timestamp time.Time, contextText string) error {
			return nil
		},
		activity.RegisterOptions{Name: RecordBellActivityName},
	)
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, sessionID string) (string, error) {
			return "", nil
		},
		activity.RegisterOptions{Name: GetOutputActivityName},
	)
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, payload event.WorkflowEvent) error {
			if emitEvent != nil {
				emitEvent(payload)
			}
			return nil
		},
		activity.RegisterOptions{Name: EmitWorkflowEventActivityName},
	)
}
