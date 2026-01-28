package workflows

import (
	"time"

	"gestalt/internal/flow"

	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	FlowRouterContinueAfter    = 500
	FlowRouterDeduperSize      = 500
	FlowSendToTerminalActivity = "SendToTerminalActivity"
	FlowPostWebhookActivity    = "PostWebhookActivity"
	FlowPublishToastActivity   = "PublishToastActivity"
	FlowGetOutputTailActivity  = "GetOutputTailActivity"
	defaultFlowOutputTailLines = 50
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
				bridgeActivity(ctx, logger, match, payload)
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

func bridgeActivity(ctx workflow.Context, logger log.Logger, match flow.ActivityMatch, signal flow.EventSignal) {
	switch match.Binding.ActivityID {
	case "send_to_terminal":
		sendToTerminal(ctx, logger, match, signal)
	case "post_webhook":
		dispatchActivity(ctx, logger, FlowPostWebhookActivity, match, signal, "")
	case "toast_notification":
		dispatchActivity(ctx, logger, FlowPublishToastActivity, match, signal, "")
	default:
		logger.Warn("unknown flow activity", "activity_id", match.Binding.ActivityID)
	}
}

func sendToTerminal(ctx workflow.Context, logger log.Logger, match flow.ActivityMatch, signal flow.EventSignal) {
	outputTail := ""
	if configBool(match.Binding.Config, "include_terminal_output") {
		sessionID := signal.Fields["session_id"]
		if sessionID == "" {
			sessionID = signal.Fields["terminal_id"]
		}
		if sessionID != "" {
			lines := configInt(match.Binding.Config, "output_tail_lines", defaultFlowOutputTailLines)
			if lines > 0 {
				outputCtx := workflow.WithActivityOptions(ctx, flowOutputActivityOptions())
				if err := workflow.ExecuteActivity(outputCtx, FlowGetOutputTailActivity, sessionID, lines).Get(outputCtx, &outputTail); err != nil {
					logger.Warn("flow output tail failed", "error", err, "session_id", sessionID)
				}
			}
		}
	}
	dispatchActivity(ctx, logger, FlowSendToTerminalActivity, match, signal, outputTail)
}

func dispatchActivity(ctx workflow.Context, logger log.Logger, activityName string, match flow.ActivityMatch, signal flow.EventSignal, outputTail string) {
	activityCtx := workflow.WithActivityOptions(ctx, flowActivityOptions())
	request := flow.ActivityRequest{
		EventID:    signal.EventID,
		TriggerID:  match.Trigger.ID,
		ActivityID: match.Binding.ActivityID,
		Event:      signal.Fields,
		Config:     match.Binding.Config,
		OutputTail: outputTail,
	}
	if err := workflow.ExecuteActivity(activityCtx, activityName, request).Get(activityCtx, nil); err != nil {
		logger.Warn("flow activity failed", "error", err, "activity", activityName)
	}
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
