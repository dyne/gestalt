package api

import (
	"io/fs"
	"net/http"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
	"gestalt/internal/watcher"
)

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, staticDir string, frontendFS fs.FS, logger *logging.Logger, eventHub *watcher.EventHub) {
	// Git info is read once on boot to avoid polling; refresh can be added later.
	gitOrigin, gitBranch := loadGitInfo()
	rest := &RestHandler{
		Manager:   manager,
		Logger:    logger,
		PlanPath:  "PLAN.org",
		GitOrigin: gitOrigin,
		GitBranch: gitBranch,
	}
	if eventHub != nil {
		eventHub.Subscribe(watcher.EventTypeGitBranchChanged, func(event watcher.Event) {
			if event.Path == "" {
				return
			}
			rest.setGitBranch(event.Path)
		})
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
		Hub:       eventHub,
		AuthToken: authToken,
	})

	mux.Handle("/api/status", loggingMiddleware(logger, restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/agents", loggingMiddleware(logger, restHandler(authToken, rest.handleAgents)))
	mux.Handle("/api/agents/", loggingMiddleware(logger, restHandler(authToken, rest.handleAgentInput)))
	mux.Handle("/api/skills", loggingMiddleware(logger, restHandler(authToken, rest.handleSkills)))
	mux.Handle("/api/skills/", loggingMiddleware(logger, restHandler(authToken, rest.handleSkill)))
	mux.Handle("/api/logs", loggingMiddleware(logger, restHandler(authToken, rest.handleLogs)))
	mux.Handle("/api/terminals", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminal)))
	mux.Handle("/api/plan", loggingMiddleware(logger, jsonErrorMiddleware(rest.handlePlan)))

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
