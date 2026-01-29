package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"gestalt/internal/config"
	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"
)

type EventsSSEHandler struct {
	Manager   *terminal.Manager
	Bus       *eventtypes.Bus[watcher.Event]
	Logger    *logging.Logger
	AuthToken string
}

type sseEventEnvelope struct {
	EventType string
	Payload   any
}

type sseTypeFilter struct {
	enabled bool
	types   map[string]struct{}
}

func newSSETypeFilter(values []string) *sseTypeFilter {
	if len(values) == 0 {
		return &sseTypeFilter{}
	}
	parsed := make(map[string]struct{})
	for _, value := range values {
		for _, entry := range strings.Split(value, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			parsed[entry] = struct{}{}
		}
	}
	if len(parsed) == 0 {
		return &sseTypeFilter{}
	}
	return &sseTypeFilter{enabled: true, types: parsed}
}

func (filter *sseTypeFilter) Allows(eventType string) bool {
	if filter == nil || !filter.enabled {
		return true
	}
	_, ok := filter.types[eventType]
	return ok
}

func (h *EventsSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !requireSSEToken(w, r, h.AuthToken, h.Logger) {
		return
	}

	spanCtx, span := startSSESpan(r, "/api/events/stream")
	defer span.End()

	ctx, cancel := context.WithCancel(spanCtx)
	defer cancel()
	r = r.WithContext(ctx)

	watcherBus := h.Bus
	if watcherBus == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "event bus unavailable")
		return
	}

	if h.Manager == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "manager unavailable")
		return
	}

	configBus := config.Bus()
	if configBus == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "config events unavailable")
		return
	}

	agentBus := h.Manager.AgentBus()
	terminalBus := h.Manager.TerminalBus()
	workflowBus := h.Manager.WorkflowBus()
	if agentBus == nil || terminalBus == nil || workflowBus == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "event stream unavailable")
		return
	}

	typeFilter := newSSETypeFilter(r.URL.Query()["types"])
	limiter := &rateLimiter{}

	allowedWatcherTypes := map[string]struct{}{
		watcher.EventTypeFileChanged:      {},
		watcher.EventTypeGitBranchChanged: {},
		watcher.EventTypeWatchError:       {},
	}

	watcherEvents, cancelWatcher := watcherBus.SubscribeFiltered(func(event watcher.Event) bool {
		if _, ok := allowedWatcherTypes[event.Type]; !ok {
			return false
		}
		if !typeFilter.Allows(event.Type) {
			return false
		}
		return limiter.Allow(time.Now())
	})
	if watcherEvents == nil {
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "event stream unavailable")
		return
	}

	configEvents, cancelConfig := configBus.SubscribeFiltered(func(event eventtypes.ConfigEvent) bool {
		return typeFilter.Allows(event.Type())
	})
	if configEvents == nil {
		cancelWatcher()
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "config events unavailable")
		return
	}

	workflowEvents, cancelWorkflow := workflowBus.SubscribeFiltered(func(event eventtypes.WorkflowEvent) bool {
		return typeFilter.Allows(event.Type())
	})
	if workflowEvents == nil {
		cancelWatcher()
		cancelConfig()
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "workflow events unavailable")
		return
	}

	agentEvents, cancelAgent := agentBus.SubscribeFiltered(func(event eventtypes.AgentEvent) bool {
		return typeFilter.Allows(event.Type())
	})
	if agentEvents == nil {
		cancelWatcher()
		cancelConfig()
		cancelWorkflow()
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "agent events unavailable")
		return
	}

	terminalEvents, cancelTerminal := terminalBus.SubscribeFiltered(func(event eventtypes.TerminalEvent) bool {
		return typeFilter.Allows(event.Type())
	})
	if terminalEvents == nil {
		cancelWatcher()
		cancelConfig()
		cancelWorkflow()
		cancelAgent()
		writeSSEUnavailable(w, r, h.Logger, http.StatusInternalServerError, "terminal events unavailable")
		return
	}

	writer, err := startSSEWriter(w)
	if err != nil {
		logSSEError(h.Logger, r, sseError{
			Status:  http.StatusInternalServerError,
			Message: "event stream unavailable",
			Err:     err,
		})
		cancelWatcher()
		cancelConfig()
		cancelWorkflow()
		cancelAgent()
		cancelTerminal()
		return
	}

	output := make(chan sseEventEnvelope, 64)
	var wg sync.WaitGroup

	forwardSSE(ctx, &wg, output, watcherEvents, cancelWatcher, func(event watcher.Event) (sseEventEnvelope, bool) {
		payload := eventPayload{
			Type:      event.Type,
			Path:      event.Path,
			Timestamp: event.Timestamp,
		}
		if payload.Timestamp.IsZero() {
			payload.Timestamp = time.Now().UTC()
		}
		return sseEventEnvelope{EventType: payload.Type, Payload: payload}, true
	})

	forwardSSE(ctx, &wg, output, configEvents, cancelConfig, func(event eventtypes.ConfigEvent) (sseEventEnvelope, bool) {
		payload := configEventPayload{
			Type:       event.Type(),
			ConfigType: event.ConfigType,
			Path:       event.Path,
			ChangeType: event.ChangeType,
			Message:    event.Message,
			Timestamp:  event.Timestamp(),
		}
		if payload.Timestamp.IsZero() {
			payload.Timestamp = time.Now().UTC()
		}
		return sseEventEnvelope{EventType: payload.Type, Payload: payload}, true
	})

	forwardSSE(ctx, &wg, output, workflowEvents, cancelWorkflow, func(event eventtypes.WorkflowEvent) (sseEventEnvelope, bool) {
		payload := workflowEventPayload{
			Type:       event.Type(),
			WorkflowID: event.WorkflowID,
			SessionID:  event.SessionID,
			Timestamp:  event.Timestamp(),
			Context:    event.Context,
		}
		if payload.Timestamp.IsZero() {
			payload.Timestamp = time.Now().UTC()
		}
		return sseEventEnvelope{EventType: payload.Type, Payload: payload}, true
	})

	forwardSSE(ctx, &wg, output, agentEvents, cancelAgent, func(event eventtypes.AgentEvent) (sseEventEnvelope, bool) {
		payload := agentEventPayload{
			Type:      event.Type(),
			AgentID:   event.AgentID,
			AgentName: event.AgentName,
			Timestamp: event.Timestamp(),
			Context:   event.Context,
		}
		if payload.Timestamp.IsZero() {
			payload.Timestamp = time.Now().UTC()
		}
		return sseEventEnvelope{EventType: payload.Type, Payload: payload}, true
	})

	forwardSSE(ctx, &wg, output, terminalEvents, cancelTerminal, func(event eventtypes.TerminalEvent) (sseEventEnvelope, bool) {
		payload := terminalEventPayload{
			Type:       event.Type(),
			TerminalID: event.TerminalID,
			Timestamp:  event.Timestamp(),
			Data:       event.Data,
		}
		if payload.Timestamp.IsZero() {
			payload.Timestamp = time.Now().UTC()
		}
		return sseEventEnvelope{EventType: payload.Type, Payload: payload}, true
	})

	go func() {
		wg.Wait()
		close(output)
	}()

	runSSEStream(r, writer, sseStreamConfig[sseEventEnvelope]{
		Logger: h.Logger,
		Output: output,
		BuildPayload: func(event sseEventEnvelope) (any, bool) {
			if event.Payload == nil {
				return nil, false
			}
			return event.Payload, true
		},
	})

	cancel()
	wg.Wait()
}

func forwardSSE[T any](ctx context.Context, wg *sync.WaitGroup, output chan<- sseEventEnvelope, input <-chan T, cancel func(), build func(T) (sseEventEnvelope, bool)) {
	if input == nil {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if cancel != nil {
			defer cancel()
		}
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-input:
				if !ok {
					return
				}
				payload, ok := build(event)
				if !ok {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case output <- payload:
				}
			}
		}
	}()
}

func writeSSEUnavailable(w http.ResponseWriter, r *http.Request, logger *logging.Logger, status int, message string) {
	writer, err := startSSEWriter(w)
	if err != nil {
		writeSSEHTTPError(w, r, logger, sseError{
			Status:  status,
			Message: message,
			Err:     err,
		})
		return
	}

	_ = writer.WriteRetry(defaultSSERetryInterval)
	_ = writeSSEErrorEvent(writer, status, message)
	logSSEError(logger, r, sseError{Status: status, Message: message})
}
