package api

import (
	"net/http"

	"gestalt/internal/logging"
)

type apiError struct {
	Status     int
	Message    string
	TerminalID string
}

type apiHandler func(http.ResponseWriter, *http.Request) *apiError

func authMiddleware(token string, next apiHandler) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) *apiError {
		if !validateToken(r, token) {
			return &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
		}
		return next(w, r)
	}
}

func jsonErrorMiddleware(next apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := next(w, r); err != nil {
			writeJSONError(w, err)
		}
	}
}

func loggingMiddleware(logger *logging.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if logger != nil {
			logger.Debug("api request", map[string]string{
				"method": r.Method,
				"path":   r.URL.Path,
			})
		}
		next.ServeHTTP(w, r)
	})
}

func methodNotAllowed(w http.ResponseWriter, allow string) *apiError {
	w.Header().Set("Allow", allow)
	return &apiError{Status: http.StatusMethodNotAllowed, Message: "method not allowed"}
}

func restHandler(token string, handler apiHandler) http.HandlerFunc {
	return jsonErrorMiddleware(authMiddleware(token, handler))
}
