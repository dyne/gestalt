package flow

import (
	"context"
	"errors"
	"sync"
	"time"

	eventpkg "gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/watcher"
)

const (
	defaultWatcherRateLimit = 60
	defaultWatcherWindow    = time.Minute
	defaultWatcherDedupTTL  = 500 * time.Millisecond
)

type Bridge struct {
	Service      *Service
	Logger       *logging.Logger
	WatcherBus   *eventpkg.Bus[watcher.Event]
	ConfigBus    *eventpkg.Bus[eventpkg.ConfigEvent]
	AgentBus     *eventpkg.Bus[eventpkg.AgentEvent]
	TerminalBus  *eventpkg.Bus[eventpkg.TerminalEvent]
	WorkflowBus  *eventpkg.Bus[eventpkg.WorkflowEvent]
	watcherCheck *watcherFilter
}

type BridgeOptions struct {
	Service     *Service
	Logger      *logging.Logger
	WatcherBus  *eventpkg.Bus[watcher.Event]
	ConfigBus   *eventpkg.Bus[eventpkg.ConfigEvent]
	AgentBus    *eventpkg.Bus[eventpkg.AgentEvent]
	TerminalBus *eventpkg.Bus[eventpkg.TerminalEvent]
	WorkflowBus *eventpkg.Bus[eventpkg.WorkflowEvent]
}

func NewBridge(options BridgeOptions) *Bridge {
	return &Bridge{
		Service:      options.Service,
		Logger:       options.Logger,
		WatcherBus:   options.WatcherBus,
		ConfigBus:    options.ConfigBus,
		AgentBus:     options.AgentBus,
		TerminalBus:  options.TerminalBus,
		WorkflowBus:  options.WorkflowBus,
		watcherCheck: newWatcherFilter(),
	}
}

func (bridge *Bridge) Start(ctx context.Context) error {
	if bridge == nil || bridge.Service == nil {
		return errors.New("flow bridge requires flow service")
	}
	if bridge.WatcherBus == nil || bridge.ConfigBus == nil || bridge.AgentBus == nil || bridge.TerminalBus == nil || bridge.WorkflowBus == nil {
		return errors.New("flow bridge requires event buses")
	}

	watcherEvents, cancelWatcher := bridge.WatcherBus.SubscribeFiltered(func(event watcher.Event) bool {
		return bridge.watcherCheck.Allows(event, time.Now())
	})
	if watcherEvents == nil {
		return errors.New("flow bridge watcher subscription failed")
	}
	configEvents, cancelConfig := bridge.ConfigBus.Subscribe()
	if configEvents == nil {
		cancelWatcher()
		return errors.New("flow bridge config subscription failed")
	}
	workflowEvents, cancelWorkflow := bridge.WorkflowBus.Subscribe()
	if workflowEvents == nil {
		cancelWatcher()
		cancelConfig()
		return errors.New("flow bridge workflow subscription failed")
	}
	agentEvents, cancelAgent := bridge.AgentBus.Subscribe()
	if agentEvents == nil {
		cancelWatcher()
		cancelConfig()
		cancelWorkflow()
		return errors.New("flow bridge agent subscription failed")
	}
	terminalEvents, cancelTerminal := bridge.TerminalBus.Subscribe()
	if terminalEvents == nil {
		cancelWatcher()
		cancelConfig()
		cancelWorkflow()
		cancelAgent()
		return errors.New("flow bridge terminal subscription failed")
	}

	go bridge.forwardWatcherEvents(ctx, watcherEvents, cancelWatcher)
	go bridge.forwardConfigEvents(ctx, configEvents, cancelConfig)
	go bridge.forwardWorkflowEvents(ctx, workflowEvents, cancelWorkflow)
	go bridge.forwardAgentEvents(ctx, agentEvents, cancelAgent)
	go bridge.forwardTerminalEvents(ctx, terminalEvents, cancelTerminal)

	return nil
}

func (bridge *Bridge) forwardWatcherEvents(ctx context.Context, events <-chan watcher.Event, cancel func()) {
	defer cancel()
	forwardLoop(ctx, events, func(event watcher.Event) {
		bridge.signalEvent(ctx, NormalizeWatcherEvent(event))
	})
}

func (bridge *Bridge) forwardConfigEvents(ctx context.Context, events <-chan eventpkg.ConfigEvent, cancel func()) {
	defer cancel()
	forwardLoop(ctx, events, func(event eventpkg.ConfigEvent) {
		bridge.signalEvent(ctx, NormalizeConfigEvent(event))
	})
}

func (bridge *Bridge) forwardWorkflowEvents(ctx context.Context, events <-chan eventpkg.WorkflowEvent, cancel func()) {
	defer cancel()
	forwardLoop(ctx, events, func(event eventpkg.WorkflowEvent) {
		bridge.signalEvent(ctx, NormalizeWorkflowEvent(event))
	})
}

func (bridge *Bridge) forwardAgentEvents(ctx context.Context, events <-chan eventpkg.AgentEvent, cancel func()) {
	defer cancel()
	forwardLoop(ctx, events, func(event eventpkg.AgentEvent) {
		bridge.signalEvent(ctx, NormalizeAgentEvent(event))
	})
}

func (bridge *Bridge) forwardTerminalEvents(ctx context.Context, events <-chan eventpkg.TerminalEvent, cancel func()) {
	defer cancel()
	forwardLoop(ctx, events, func(event eventpkg.TerminalEvent) {
		bridge.signalEvent(ctx, NormalizeTerminalEvent(event))
	})
}

func forwardLoop[T any](ctx context.Context, events <-chan T, handle func(T)) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			handle(event)
		}
	}
}

func (bridge *Bridge) signalEvent(ctx context.Context, normalized map[string]string) {
	if bridge == nil || bridge.Service == nil {
		return
	}
	if err := bridge.Service.SignalEvent(ctx, normalized, ""); err != nil {
		bridge.logWarn("flow router signal failed", map[string]string{
			"error": err.Error(),
		})
	}
}

func (bridge *Bridge) logWarn(message string, fields map[string]string) {
	if bridge == nil || bridge.Logger == nil {
		return
	}
	bridge.Logger.Warn(message, fields)
}

type watcherFilter struct {
	mutex   sync.Mutex
	allowed map[string]struct{}
	limiter *bridgeLimiter
	deduper *watcherDeduper
}

func newWatcherFilter() *watcherFilter {
	return &watcherFilter{
		allowed: map[string]struct{}{
			watcher.EventTypeFileChanged:      {},
			watcher.EventTypeGitBranchChanged: {},
		},
		limiter: newBridgeLimiter(defaultWatcherRateLimit, defaultWatcherWindow),
		deduper: &watcherDeduper{window: defaultWatcherDedupTTL},
	}
}

func (filter *watcherFilter) Allows(event watcher.Event, now time.Time) bool {
	if filter == nil {
		return true
	}
	filter.mutex.Lock()
	defer filter.mutex.Unlock()
	if _, ok := filter.allowed[event.Type]; !ok {
		return false
	}
	if filter.limiter != nil && !filter.limiter.Allow(now) {
		return false
	}
	if filter.deduper != nil && !filter.deduper.Allow(event, now) {
		return false
	}
	return true
}
