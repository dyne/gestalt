package workflows

import (
	"gestalt/internal/flow"

	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"
)

func FlowDispatchWorkflow(ctx workflow.Context, request flow.ActivityRequest) error {
	logger := workflow.GetLogger(ctx)

	switch request.ActivityID {
	case "send_to_terminal":
		if request.OutputTail == "" {
			request.OutputTail = buildOutputTail(ctx, logger, request)
		}
		return executeFlowActivity(ctx, logger, FlowSendToTerminalActivity, request)
	case "post_webhook":
		return executeFlowActivity(ctx, logger, FlowPostWebhookActivity, request)
	case "toast_notification":
		return executeFlowActivity(ctx, logger, FlowPublishToastActivity, request)
	case "spawn_agent_session":
		return executeFlowActivity(ctx, logger, FlowSpawnAgentSessionActivity, request)
	default:
		logger.Warn("unknown flow activity", "activity_id", request.ActivityID)
		return nil
	}
}

func buildOutputTail(ctx workflow.Context, logger log.Logger, request flow.ActivityRequest) string {
	if !configBool(request.Config, "include_terminal_output") {
		return ""
	}
	sessionID := request.Event["session_id"]
	if sessionID == "" {
		sessionID = request.Event["terminal_id"]
	}
	if sessionID == "" {
		return ""
	}
	lines := configInt(request.Config, "output_tail_lines", defaultFlowOutputTailLines)
	if lines <= 0 {
		return ""
	}
	outputCtx := workflow.WithActivityOptions(ctx, flowOutputActivityOptions())
	var outputTail string
	if err := workflow.ExecuteActivity(outputCtx, FlowGetOutputTailActivity, sessionID, lines).Get(outputCtx, &outputTail); err != nil {
		logger.Warn("flow output tail failed", "error", err, "session_id", sessionID)
		return ""
	}
	return outputTail
}

func executeFlowActivity(ctx workflow.Context, logger log.Logger, activityName string, request flow.ActivityRequest) error {
	activityCtx := workflow.WithActivityOptions(ctx, flowActivityOptions())
	if err := workflow.ExecuteActivity(activityCtx, activityName, request).Get(activityCtx, nil); err != nil {
		logger.Warn("flow activity failed", "error", err, "activity", activityName)
	}
	return nil
}
