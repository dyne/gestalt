package api

import (
	"io/fs"
	"net/http"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"
)

type StatusConfig struct {
	TemporalUIPort int
}

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, statusConfig StatusConfig, scipIndexPath string, scipAutoReindex bool, staticDir string, frontendFS fs.FS, logger *logging.Logger, eventBus *event.Bus[watcher.Event]) {
	// Git info is read once on boot to avoid polling; refresh can be added later.
	gitOrigin, gitBranch := loadGitInfo()
	planPath := plan.DefaultPath()
	planCache := plan.NewCache(planPath, logger)
	rest := &RestHandler{
		Manager:        manager,
		Logger:         logger,
		PlanPath:       planPath,
		PlanCache:      planCache,
		GitOrigin:      gitOrigin,
		GitBranch:      gitBranch,
		TemporalUIPort: statusConfig.TemporalUIPort,
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
		AuthToken: authToken,
	})
	mux.Handle("/ws/logs", &LogsHandler{
		Logger:    logger,
		AuthToken: authToken,
	})
	mux.Handle("/ws/events", &EventsHandler{
		Bus:       eventBus,
		AuthToken: authToken,
	})
	mux.Handle("/api/agents/events", &AgentEventsHandler{
		Manager:   manager,
		AuthToken: authToken,
	})
	mux.Handle("/api/terminals/events", &TerminalEventsHandler{
		Manager:   manager,
		AuthToken: authToken,
	})
	mux.Handle("/api/workflows/events", &WorkflowEventsHandler{
		Manager:   manager,
		AuthToken: authToken,
	})
	mux.Handle("/api/config/events", &ConfigEventsHandler{
		AuthToken: authToken,
	})

	mux.Handle("/api/status", loggingMiddleware(logger, restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/metrics", loggingMiddleware(logger, restHandler(authToken, rest.handleMetrics)))
	mux.Handle("/api/events/debug", loggingMiddleware(logger, restHandler(authToken, rest.handleEventDebug)))
	mux.Handle("/api/workflows", loggingMiddleware(logger, restHandler(authToken, rest.handleWorkflows)))
	mux.Handle("/api/agents", loggingMiddleware(logger, restHandler(authToken, rest.handleAgents)))
	mux.Handle("/api/agents/", loggingMiddleware(logger, restHandler(authToken, rest.handleAgentInput)))
	mux.Handle("/api/skills", loggingMiddleware(logger, restHandler(authToken, rest.handleSkills)))
	mux.Handle("/api/skills/", loggingMiddleware(logger, restHandler(authToken, rest.handleSkill)))
	mux.Handle("/api/logs", loggingMiddleware(logger, restHandler(authToken, rest.handleLogs)))
	mux.Handle("/api/terminals", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminal)))
	mux.Handle("/api/plan", loggingMiddleware(logger, jsonErrorMiddleware(rest.handlePlan)))
	mux.Handle("/api/plan/current", loggingMiddleware(logger, jsonErrorMiddleware(rest.handlePlanCurrent)))

	if scipIndexPath != "" {
		scipHandler, err := NewSCIPHandler(scipIndexPath, logger, SCIPHandlerOptions{
			AutoReindex:        scipAutoReindex,
			AutoReindexOnStart: scipAutoReindex,
			EventBus:           eventBus,
		})
		if err != nil {
			if logger != nil {
				logger.Warn("scip handler unavailable", map[string]string{
					"error": err.Error(),
				})
			}
		} else {
			mux.Handle("/api/scip/status", loggingMiddleware(logger, restHandler(authToken, scipHandler.Status)))
			mux.Handle("/api/scip/symbols", loggingMiddleware(logger, restHandler(authToken, scipHandler.FindSymbols)))
			mux.Handle("/api/scip/symbols/", loggingMiddleware(logger, restHandler(authToken, scipHandler.HandleSymbol)))
			mux.Handle("/api/scip/files/", loggingMiddleware(logger, restHandler(authToken, scipHandler.GetFileSymbols)))
			mux.Handle("/api/scip/index", loggingMiddleware(logger, restHandler(authToken, scipHandler.ReIndex)))
		}
	}

	if staticDir != "" {
		mux.Handle("/", NewSPAHandler(staticDir))
		return
	}

	if frontendFS != nil {
		mux.Handle("/", NewSPAHandlerFS(frontendFS))
		return
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})
}
