package api

import (
	"io/fs"
	"net/http"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/otel"
	"gestalt/internal/plan"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"

	otelapi "go.opentelemetry.io/otel"
)

type StatusConfig struct {
	TemporalUIPort int
}

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, statusConfig StatusConfig, scipIndexPath string, scipAutoReindex bool, staticDir string, frontendFS fs.FS, logger *logging.Logger, eventBus *event.Bus[watcher.Event], scipEventBus *event.Bus[event.SCIPEvent]) *SCIPHandler {
	// Git info is read once on boot to avoid polling; refresh can be added later.
	gitOrigin, gitBranch := loadGitInfo()
	planPath := plan.DefaultPath()
	planCache := plan.NewCache(planPath, logger)
	metricsSummary := otel.NewAPISummaryStore()
	rest := &RestHandler{
		Manager:        manager,
		Logger:         logger,
		MetricsSummary: metricsSummary,
		PlanPath:       planPath,
		PlanCache:      planCache,
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
		if planCache != nil {
			planEvents, _ := eventBus.SubscribeFiltered(func(event watcher.Event) bool {
				return event.Type == watcher.EventTypeFileChanged
			})
			go func() {
				for event := range planEvents {
					if !planCache.MatchesPath(event.Path) {
						continue
					}
					if _, err := planCache.Reload(); err != nil && logger != nil {
						logger.Warn("plan cache reload failed", map[string]string{
							"path":  event.Path,
							"error": err.Error(),
						})
					}
				}
			}()
		}
	}

	mux.Handle("/ws/terminal/", &TerminalHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/ws/logs", &LogsHandler{
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/ws/events", &EventsHandler{
		Bus:       eventBus,
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/api/agents/events", &AgentEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/api/terminals/events", &TerminalEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/api/workflows/events", &WorkflowEventsHandler{
		Manager:   manager,
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/api/config/events", &ConfigEventsHandler{
		Logger:    logger,
		AuthToken: authToken,
	})

	mux.Handle("/api/status", wrap("/api/status", "status", "read", restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/metrics", wrap("/api/metrics", "status", "read", restHandler(authToken, rest.handleMetrics)))
	mux.Handle("/api/metrics/summary", wrap("/api/metrics/summary", "status", "query", restHandler(authToken, rest.handleMetricsSummary)))
	mux.Handle("/api/events/debug", wrap("/api/events/debug", "status", "query", restHandler(authToken, rest.handleEventDebug)))
	mux.Handle("/api/workflows", wrap("/api/workflows", "workflows", "read", restHandler(authToken, rest.handleWorkflows)))
	mux.Handle("/api/agents", wrap("/api/agents", "agents", "read", restHandler(authToken, rest.handleAgents)))
	mux.Handle("/api/agents/", wrap("/api/agents/:name/input", "agents", "stream", restHandler(authToken, rest.handleAgentInput)))
	mux.Handle("/api/skills", wrap("/api/skills", "skills", "read", restHandler(authToken, rest.handleSkills)))
	mux.Handle("/api/skills/", wrap("/api/skills/:name", "skills", "read", restHandler(authToken, rest.handleSkill)))
	mux.Handle("/api/logs", wrap("/api/logs", "logs", "read", restHandler(authToken, rest.handleLogs)))
	mux.Handle("/api/otel/logs", wrap("/api/otel/logs", "logs", "query", restHandler(authToken, rest.handleOTelLogs)))
	mux.Handle("/api/otel/traces", wrap("/api/otel/traces", "traces", "query", restHandler(authToken, rest.handleOTelTraces)))
	mux.Handle("/api/otel/metrics", wrap("/api/otel/metrics", "metrics", "query", restHandler(authToken, rest.handleOTelMetrics)))
	mux.Handle("/api/terminals", wrap("/api/terminals", "terminals", "auto", restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", wrap("/api/terminals/:id", "terminals", "auto", restHandler(authToken, rest.handleTerminal)))
	mux.Handle("/api/plan", wrap("/api/plan", "plan", "read", jsonErrorMiddleware(rest.handlePlan)))
	mux.Handle("/api/plan/current", wrap("/api/plan/current", "plan", "read", jsonErrorMiddleware(rest.handlePlanCurrent)))

	var scipHandler *SCIPHandler
	if scipIndexPath != "" {
		handler, err := NewSCIPHandler(scipIndexPath, logger, SCIPHandlerOptions{
			AutoReindex:        scipAutoReindex,
			AutoReindexOnStart: scipAutoReindex,
			EventBus:           eventBus,
			SCIPEventBus:       scipEventBus,
		})
		if err != nil {
			if logger != nil {
				logger.Warn("scip handler unavailable", map[string]string{
					"error": err.Error(),
				})
			}
		} else {
			scipHandler = handler
			mux.Handle("/api/scip/status", wrap("/api/scip/status", "config", "read", restHandler(authToken, handler.Status)))
			mux.Handle("/api/scip/symbols", wrap("/api/scip/symbols", "config", "read", restHandler(authToken, handler.FindSymbols)))
			mux.Handle("/api/scip/symbols/", wrap("/api/scip/symbols/:id", "config", "read", restHandler(authToken, handler.HandleSymbol)))
			mux.Handle("/api/scip/files/", wrap("/api/scip/files/:path", "config", "read", restHandler(authToken, handler.GetFileSymbols)))
			mux.Handle("/api/scip/index", wrap("/api/scip/index", "config", "update", restHandler(authToken, handler.ReIndex)))
			mux.Handle("/api/scip/reindex", wrap("/api/scip/reindex", "config", "update", restHandler(authToken, handler.Reindex)))
			if scipEventBus != nil {
				mux.Handle("/api/scip/events", &SCIPEventsHandler{
					Logger:    logger,
					AuthToken: authToken,
					Bus:       scipEventBus,
				})
			}
		}
	}

	if staticDir != "" {
		mux.Handle("/", NewSPAHandler(staticDir))
		return scipHandler
	}

	if frontendFS != nil {
		mux.Handle("/", NewSPAHandlerFS(frontendFS))
		return scipHandler
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})

	return scipHandler
}
