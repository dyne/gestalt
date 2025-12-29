package api

import (
	"net/http"

	"gestalt/internal/logging"
	"gestalt/internal/terminal"
)

func RegisterRoutes(mux *http.ServeMux, manager *terminal.Manager, authToken string, staticDir string, logger *logging.Logger) {
	rest := &RestHandler{
		Manager: manager,
		Logger:  logger,
	}

	mux.Handle("/ws/terminal/", &TerminalHandler{
		Manager:   manager,
		AuthToken: authToken,
	})
	mux.Handle("/ws/logs", &LogsHandler{
		Logger:    logger,
		AuthToken: authToken,
	})

	mux.Handle("/api/status", loggingMiddleware(logger, restHandler(authToken, rest.handleStatus)))
	mux.Handle("/api/agents", loggingMiddleware(logger, restHandler(authToken, rest.handleAgents)))
	mux.Handle("/api/logs", loggingMiddleware(logger, restHandler(authToken, rest.handleLogs)))
	mux.Handle("/api/terminals", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminals)))
	mux.Handle("/api/terminals/", loggingMiddleware(logger, restHandler(authToken, rest.handleTerminal)))

	if staticDir != "" {
		mux.Handle("/", NewSPAHandler(staticDir))
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
