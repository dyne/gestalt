package workflows

import (
	"context"
	"testing"

	"gestalt/internal/flow"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestFlowDispatchWorkflowSendToTerminal(t *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()
	workflowEnvironment.RegisterWorkflow(FlowDispatchWorkflow)

	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, sessionID string, lines int) (string, error) {
			if sessionID != "session-1" {
				t.Fatalf("unexpected session id: %q", sessionID)
			}
			if lines != 12 {
				t.Fatalf("unexpected lines: %d", lines)
			}
			return "tail output", nil
		},
		activity.RegisterOptions{Name: FlowGetOutputTailActivity},
	)

	var captured flow.ActivityRequest
	workflowEnvironment.RegisterActivityWithOptions(
		func(ctx context.Context, request flow.ActivityRequest) error {
			captured = request
			return nil
		},
		activity.RegisterOptions{Name: FlowSendToTerminalActivity},
	)

	request := flow.ActivityRequest{
		EventID:    "event-1",
		TriggerID:  "trigger-1",
		ActivityID: "send_to_terminal",
		Event: map[string]string{
			"session_id": "session-1",
		},
		Config: map[string]any{
			"include_terminal_output": true,
			"output_tail_lines":       12,
			"message_template":        "hello",
		},
	}

	workflowEnvironment.ExecuteWorkflow(FlowDispatchWorkflow, request)
	if !workflowEnvironment.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if workflowEnvironment.GetWorkflowError() != nil {
		t.Fatalf("workflow error: %v", workflowEnvironment.GetWorkflowError())
	}
	if captured.OutputTail != "tail output" {
		t.Fatalf("expected output tail, got %q", captured.OutputTail)
	}
}
