package api

import (
	"io/fs"
	"net/http"
	"strings"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/otel"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"

	otelapi "go.opentelemetry.io/otel"
)

type StatusConfig struct {
	TemporalUIPort int
}

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, statusConfig StatusConfig, staticDir string, frontendFS fs.FS, logger *logging.Logger, eventBus *event.Bus[watcher.Event]) {
	// Git info is read once on boot to avoid polling; refresh can be added later.
	gitOrigin, gitBranch := loadGitInfo()
	metricsSummary := otel.NewAPISummaryStore()
	rest := &RestHandler{
		Manager:        manager,
		Logger:         logger,
		MetricsSummary: metricsSummary,
		GitOrigin:      gitOrigin,
		GitBranch:      gitBranch,
		TemporalUIPort: statusConfig.TemporalUIPort,
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

	mux.Handle("/ws/terminal/", securityHeadersMiddleware(cacheControlNoStore, &TerminalHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/ws/logs", securityHeadersMiddleware(cacheControlNoStore, &LogsHandler{
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
	mux.Handle("/api/terminals/events", securityHeadersMiddleware(cacheControlNoStore, &TerminalEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/workflows/events", securityHeadersMiddleware(cacheControlNoStore, &WorkflowEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	}))
	mux.Handle("/api/config/events", securityHeadersMiddleware(cacheControlNoStore, &ConfigEventsHandler{
		Logger:    logger,
		AuthToken: authToken,
	}))

	mux.Handle("/api/status", wrap("/api/status", "status", "read", restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/metrics/summary", wrap("/api/metrics/summary", "status", "query", restHandler(authToken, rest.handleMetricsSummary)))
	mux.Handle("/api/workflows", wrap("/api/workflows", "workflows", "read", restHandler(authToken, rest.handleWorkflows)))
	mux.Handle("/api/agents", wrap("/api/agents", "agents", "read", restHandler(authToken, rest.handleAgents)))
	agentInputHandler := wrap("/api/agents/:name/input", "agents", "stream", restHandler(authToken, rest.handleAgentInput))
	agentSendInputHandler := wrap("/api/agents/:name/send-input", "agents", "stream", restHandler(authToken, rest.handleAgentSendInput))
	mux.Handle("/api/agents/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if strings.HasSuffix(path, "/send-input") {
			agentSendInputHandler.ServeHTTP(w, r)
			return
		}
		agentInputHandler.ServeHTTP(w, r)
	}))
	mux.Handle("/api/skills", wrap("/api/skills", "skills", "read", restHandler(authToken, rest.handleSkills)))
	mux.Handle("/api/otel/logs", wrap("/api/otel/logs", "logs", "create", restHandler(authToken, rest.handleOTelLogs)))
	mux.Handle("/api/otel/traces", wrap("/api/otel/traces", "traces", "query", restHandler(authToken, rest.handleOTelTraces)))
	mux.Handle("/api/otel/metrics", wrap("/api/otel/metrics", "metrics", "query", restHandler(authToken, rest.handleOTelMetrics)))
	mux.Handle("/api/terminals", wrap("/api/terminals", "terminals", "auto", restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", wrap("/api/terminals/:id", "terminals", "auto", restHandler(authToken, rest.handleTerminal)))
	mux.Handle("/api/plans", wrap("/api/plans", "plan", "read", restHandler(authToken, rest.handlePlansList)))

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
