package api

import (
	"net/http"
	"time"
)

func (h *RestHandler) handleMetricsSummary(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if h.MetricsSummary == nil {
		return &apiError{Status: http.StatusServiceUnavailable, Message: "metrics summary unavailable"}
	}
	summary := h.MetricsSummary.Summary(time.Now().UTC())
	writeJSON(w, http.StatusOK, summary)
	return nil
}
