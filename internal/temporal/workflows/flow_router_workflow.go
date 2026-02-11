package workflows

import (
	"strings"
	"time"

	"gestalt/internal/flow"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	FlowRouterContinueAfter       = 500
	FlowRouterDeduperSize         = 500
	FlowSendToTerminalActivity    = "SendToTerminalActivity"
	FlowPostWebhookActivity       = "PostWebhookActivity"
	FlowPublishToastActivity      = "PublishToastActivity"
	FlowSpawnAgentSessionActivity = "SpawnAgentSessionActivity"
	FlowGetOutputTailActivity     = "GetOutputTailActivity"
	defaultFlowOutputTailLines    = 50
)

func FlowRouterWorkflow(ctx workflow.Context, initialConfig flow.Config) error {
	logger := workflow.GetLogger(ctx)
	config := normalizeFlowConfig(initialConfig)
	deduper := flow.NewEventDeduper(FlowRouterDeduperSize)
	eventCount := 0

	configSignals := workflow.GetSignalChannel(ctx, flow.RouterWorkflowConfigSignal)
	eventSignals := workflow.GetSignalChannel(ctx, flow.RouterWorkflowEventSignal)

	for {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(configSignals, func(channel workflow.ReceiveChannel, _ bool) {
			var payload flow.Config
			channel.Receive(ctx, &payload)
			config = normalizeFlowConfig(payload)
		})
		selector.AddReceive(eventSignals, func(channel workflow.ReceiveChannel, _ bool) {
			var payload flow.EventSignal
			channel.Receive(ctx, &payload)
			if payload.EventID == "" {
				payload.EventID = flow.BuildEventID(payload.Fields)
			}
			if payload.EventID == "" {
				return
			}
			if deduper.Seen(payload.EventID) {
				return
			}
			eventCount++
			flowMatches := flow.MatchBindings(config, payload.Fields)
			for _, match := range flowMatches {
				startChildWorkflow(ctx, logger, match, payload)
			}
		})
		selector.Select(ctx)

		if eventCount >= FlowRouterContinueAfter {
			return workflow.NewContinueAsNewError(ctx, FlowRouterWorkflow, config)
		}
	}
}

func normalizeFlowConfig(cfg flow.Config) flow.Config {
	if cfg.Version == 0 {
		cfg = flow.DefaultConfig()
	}
	if cfg.Triggers == nil {
		cfg.Triggers = []flow.EventTrigger{}
	}
	if cfg.BindingsByTriggerID == nil {
		cfg.BindingsByTriggerID = map[string][]flow.ActivityBinding{}
	}
	return cfg
}

func startChildWorkflow(ctx workflow.Context, logger log.Logger, match flow.ActivityMatch, signal flow.EventSignal) {
	request := flow.ActivityRequest{
		EventID:    signal.EventID,
		TriggerID:  match.Trigger.ID,
		ActivityID: match.Binding.ActivityID,
		Event:      signal.Fields,
		Config:     match.Binding.Config,
	}
	childID := buildFlowChildWorkflowID(signal.EventID, match.Trigger.ID, match.Binding.ActivityID)
	childOptions := workflow.ChildWorkflowOptions{
		WorkflowID:        childID,
		TaskQueue:         SessionTaskQueueName,
		ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
	}
	childCtx := workflow.WithChildOptions(ctx, childOptions)
	childFuture := workflow.ExecuteChildWorkflow(childCtx, FlowDispatchWorkflow, request)
	var execution workflow.Execution
	if err := childFuture.GetChildWorkflowExecution().Get(childCtx, &execution); err != nil {
		if temporal.IsWorkflowExecutionAlreadyStartedError(err) {
			return
		}
		logger.Warn("flow child workflow start failed", "error", err, "workflow_id", childID)
	}
}

func buildFlowChildWorkflowID(eventID, triggerID, activityID string) string {
	parts := []string{
		"flow",
		strings.TrimSpace(eventID),
		strings.TrimSpace(triggerID),
		strings.TrimSpace(activityID),
	}
	return strings.Join(parts, "/")
}

func flowActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: DefaultActivityTimeout,
		HeartbeatTimeout:    DefaultActivityHeartbeat,
		RetryPolicy:         flowActivityRetryPolicy(),
	}
}

func flowOutputActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: ReadOutputTimeout,
		RetryPolicy:         flowActivityRetryPolicy(),
	}
}

func flowActivityRetryPolicy() *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:    time.Second,
		BackoffCoefficient: 2,
		MaximumInterval:    30 * time.Second,
		MaximumAttempts:    DefaultActivityRetryAttempts,
	}
}

func configBool(config map[string]any, key string) bool {
	if config == nil {
		return false
	}
	value, ok := config[key]
	if !ok || value == nil {
		return false
	}
	if parsed, ok := value.(bool); ok {
		return parsed
	}
	return false
}

func configInt(config map[string]any, key string, fallback int) int {
	if config == nil {
		return fallback
	}
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch parsed := value.(type) {
	case int:
		return parsed
	case int32:
		return int(parsed)
	case int64:
		return int(parsed)
	case float32:
		return int(parsed)
	case float64:
		return int(parsed)
	default:
		return fallback
	}
}
