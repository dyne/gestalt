package api

import (
	"net/http"

	"gestalt/internal/logging"
	"gestalt/internal/otel"
)

type apiError struct {
	Status     int
	Message    string
	Code       string
	TerminalID string
}

type apiHandler func(http.ResponseWriter, *http.Request) *apiError

func authMiddleware(token string, next apiHandler) apiHandler {
	return func(w http.ResponseWriter, r *http.Request) *apiError {
		if !validateToken(r, token) {
			otel.RecordSpanEvent(r.Context(), "auth.token_rejected")
			return &apiError{Status: http.StatusUnauthorized, Message: "unauthorized"}
		}
		otel.RecordSpanEvent(r.Context(), "auth.token_validated")
		return next(w, r)
	}
}

func jsonErrorMiddleware(next apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := next(w, r); err != nil {
			code := err.Code
			if code == "" {
				code = errorCodeForStatus(err.Status)
			}
			otel.RecordAPIError(r.Context(), otel.APIErrorInfo{
				Status:  err.Status,
				Code:    code,
				Message: err.Message,
			})
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
