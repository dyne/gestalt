package api

import (
	"io/fs"
	"net/http"

	"gestalt/internal/event"
	"gestalt/internal/flow"
	flowruntime "gestalt/internal/flow/runtime"
	"gestalt/internal/gitlog"
	"gestalt/internal/logging"
	"gestalt/internal/notify"
	"gestalt/internal/otel"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"

	otelapi "go.opentelemetry.io/otel"
)

type StatusConfig struct {
	SessionScrollbackLines int
	SessionFontFamily      string
	SessionFontSize        string
	SessionInputFontFamily string
	SessionInputFontSize   string
}

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, statusConfig StatusConfig, staticDir string, frontendFS fs.FS, logger *logging.Logger, eventBus *event.Bus[watcher.Event], flowService *flow.Service) {
	// Git info is read once on boot to avoid polling; refresh can be added later.
	gitOrigin, gitBranch := loadGitInfo()
	metricsSummary := otel.NewAPISummaryStore()
	notificationSink := notify.NewOTelSink(nil)
	if flowService == nil {
		flowRepo := flow.NewFileRepository(flow.DefaultConfigPath(), logger)
		var dispatcher flow.Dispatcher
		if manager != nil {
			dispatcher = flowruntime.NewDispatcher(manager, logger, notificationSink, 0)
		}
		flowService = flow.NewService(flowRepo, dispatcher, logger)
	}
	rest := &RestHandler{
		Manager:                manager,
		FlowService:            flowService,
		NotificationSink:       notificationSink,
		Logger:                 logger,
		MetricsSummary:         metricsSummary,
		GitLogReader:           gitlog.GitCmdReader{},
		GitOrigin:              gitOrigin,
		GitBranch:              gitBranch,
		SessionScrollbackLines: statusConfig.SessionScrollbackLines,
		SessionFontFamily:      statusConfig.SessionFontFamily,
		SessionFontSize:        statusConfig.SessionFontSize,
		SessionInputFontFamily: statusConfig.SessionInputFontFamily,
		SessionInputFontSize:   statusConfig.SessionInputFontSize,
	}
	meter := otelapi.GetMeterProvider().Meter("gestalt/api")
	tracer := otelapi.Tracer("gestalt/api")
	instrument, err := otel.NewAPIInstrumentationMiddleware(meter,
		otel.WithAPITracer(tracer),
		otel.WithAPIResolver(apiAgentResolver(manager)),
		otel.WithSummaryStore(metricsSummary),
	)
	if err != nil && logger != nil {
		logger.Warn("otel api middleware unavailable", map[string]string{
			"error": err.Error(),
		})
	}
	if instrument == nil {
		instrument = func(next http.Handler) http.Handler { return next }
	}
	wrap := func(route, category, operation string, handler http.Handler) http.Handler {
		return otel.WithRouteInfo(instrument(loggingMiddleware(logger, handler)), otel.RouteInfo{
			Route:     route,
			Category:  category,
			Operation: operation,
		})
	}
	if eventBus != nil {
		gitEvents, _ := eventBus.SubscribeFiltered(func(event watcher.Event) bool {
			return event.Type == watcher.EventTypeGitBranchChanged
		})
		go func() {
			for event := range gitEvents {
				if event.Path == "" {
					continue
				}
				rest.setGitBranch(event.Path)
			}
		}()
	}

	mux.Handle("/ws/session/", securityHeadersMiddleware(cacheControlNoStore, &TerminalHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/ws/logs", securityHeadersMiddleware(cacheControlNoStore, &LogsHandler{
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/logs/stream", securityHeadersMiddleware(cacheControlNoStore, &LogsSSEHandler{
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/notifications/stream", securityHeadersMiddleware(cacheControlNoStore, &NotificationsSSEHandler{
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/ws/events", securityHeadersMiddleware(cacheControlNoStore, &EventsHandler{
		Bus:       eventBus,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/events/stream", securityHeadersMiddleware(cacheControlNoStore, &EventsSSEHandler{
		Manager:   manager,
		Bus:       eventBus,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/agents/events", securityHeadersMiddleware(cacheControlNoStore, &AgentEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/sessions/events", securityHeadersMiddleware(cacheControlNoStore, &TerminalEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/config/events", securityHeadersMiddleware(cacheControlNoStore, &ConfigEventsHandler{
		Logger:    logger,
		AuthToken: authToken,
	}))

	mux.Handle("/api/status", wrap("/api/status", "status", "read", restHandler(authToken, logger, rest.handleStatus)))
	mux.Handle("/api/metrics/summary", wrap("/api/metrics/summary", "status", "query", restHandler(authToken, logger, rest.handleMetricsSummary)))
	mux.Handle("/api/git/log", wrap("/api/git/log", "status", "query", restHandler(authToken, logger, rest.handleGitLog)))
	mux.Handle("/api/agents", wrap("/api/agents", "agents", "read", restHandler(authToken, logger, rest.handleAgents)))
	mux.Handle("/api/skills", wrap("/api/skills", "skills", "read", restHandler(authToken, logger, rest.handleSkills)))
	mux.Handle("/api/otel/logs", wrap("/api/otel/logs", "logs", "create", restHandler(authToken, logger, rest.handleOTelLogs)))
	mux.Handle("/api/otel/traces", wrap("/api/otel/traces", "traces", "query", restHandler(authToken, logger, rest.handleOTelTraces)))
	mux.Handle("/api/otel/metrics", wrap("/api/otel/metrics", "metrics", "query", restHandler(authToken, logger, rest.handleOTelMetrics)))
	mux.Handle("/api/sessions", wrap("/api/sessions", "sessions", "auto", restHandler(authToken, logger, rest.handleTerminals)))
	mux.Handle("/api/sessions/", wrap("/api/sessions/:id", "sessions", "auto", restHandler(authToken, logger, rest.handleTerminal)))
	mux.Handle("/api/plans", wrap("/api/plans", "plan", "read", restHandler(authToken, logger, rest.handlePlansList)))
	mux.Handle("/api/flow/activities", wrap("/api/flow/activities", "flow", "read", restHandler(authToken, logger, rest.handleFlowActivities)))
	mux.Handle("/api/flow/event-types", wrap("/api/flow/event-types", "flow", "read", restHandler(authToken, logger, rest.handleFlowEventTypes)))
	mux.Handle("/api/flow/config", wrap("/api/flow/config", "flow", "auto", restHandler(authToken, logger, rest.handleFlowConfig)))
	mux.Handle("/api/flow/config/export", wrap("/api/flow/config/export", "flow", "read", restHandler(authToken, logger, rest.handleFlowConfigExport)))
	mux.Handle("/api/flow/config/import", wrap("/api/flow/config/import", "flow", "update", restHandler(authToken, logger, rest.handleFlowConfigImport)))
	mux.Handle("/api/", securityHeadersMiddleware(cacheControlNoStore, http.NotFoundHandler()))

	if staticDir != "" {
		mux.Handle("/", NewSPAHandler(staticDir))
		return
	}

	if frontendFS != nil {
		mux.Handle("/", NewSPAHandlerFS(frontendFS))
		return
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w, cacheControlNoCache)
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})

	return
}
