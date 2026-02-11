package workflows

import (
	"testing"

	"gestalt/internal/flow"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestFlowRouterChildWorkflowIdempotency(t *testing.T) {
	workflowTestSuite := &testsuite.WorkflowTestSuite{}
	workflowEnvironment := workflowTestSuite.NewTestWorkflowEnvironment()

	var childIDs []string
	workflowEnvironment.OnWorkflow(FlowDispatchWorkflow, mock.Anything, mock.Anything).Return(
		func(ctx workflow.Context, request flow.ActivityRequest) error {
			info := workflow.GetInfo(ctx)
			childIDs = append(childIDs, info.WorkflowExecution.ID)
			return nil
		},
	)
	workflowEnvironment.RegisterWorkflow(flowChildStartTestWorkflow)

	workflowEnvironment.ExecuteWorkflow(flowChildStartTestWorkflow)
	if !workflowEnvironment.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if workflowEnvironment.GetWorkflowError() != nil {
		t.Fatalf("workflow error: %v", workflowEnvironment.GetWorkflowError())
	}
	if len(childIDs) != 2 {
		t.Fatalf("expected 2 child starts, got %d", len(childIDs))
	}
	if childIDs[0] != childIDs[1] {
		t.Fatalf("expected deterministic child ids, got %q and %q", childIDs[0], childIDs[1])
	}
	expectedID := buildFlowChildWorkflowID("event-1", "t1", "send_to_terminal")
	if childIDs[0] != expectedID {
		t.Fatalf("expected child id %q, got %q", expectedID, childIDs[0])
	}
}

func flowChildStartTestWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	match := flow.ActivityMatch{
		Trigger: flow.EventTrigger{ID: "t1"},
		Binding: flow.ActivityBinding{
			ActivityID: "send_to_terminal",
			Config: map[string]any{
				"message_template": "hello",
			},
		},
	}
	signal := flow.EventSignal{
		EventID: "event-1",
		Fields: map[string]string{
			"session_id": "session-1",
		},
	}
	startChildWorkflow(ctx, logger, match, signal)
	startChildWorkflow(ctx, logger, match, signal)
	return nil
}
