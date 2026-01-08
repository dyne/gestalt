package workflows

import (
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
)

func TestSessionWorkflowSignals(testingContext *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()
	workflowEnvironment.RegisterWorkflow(SessionWorkflow)

	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	bellTime := time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC)

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
	if workflowResult.EventCount != 4 {
		testingContext.Fatalf("expected 4 events, got %d", workflowResult.EventCount)
	}
}

func TestSessionWorkflowAbortAction(testingContext *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()
	workflowEnvironment.RegisterWorkflow(SessionWorkflow)

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
}
