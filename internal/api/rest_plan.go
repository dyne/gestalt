package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"gestalt/internal/plan"
)

func (h *RestHandler) handlePlan(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}

	planPath := h.PlanPath
	if planPath == "" {
		planPath = plan.DefaultPath()
	}

	info, statErr := os.Stat(planPath)
	content, err := os.ReadFile(planPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if h.Logger != nil {
				h.Logger.Warn("plan file not found", map[string]string{
					"path": planPath,
				})
			}
			content = []byte{}
		} else {
			return &apiError{Status: http.StatusInternalServerError, Message: "failed to read plan file"}
		}
	}
	if statErr == nil {
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	}

	etag := planETag(content)
	w.Header().Set("ETag", etag)
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	writeJSON(w, http.StatusOK, planResponse{Content: string(content)})
	return nil
}

func (h *RestHandler) handlePlanCurrent(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, "GET")
	}
	if h.PlanCache == nil {
		return &apiError{Status: http.StatusInternalServerError, Message: "plan cache unavailable"}
	}

	currentWork, currentError := h.PlanCache.Current()
	if currentError != nil {
		if h.Logger != nil {
			h.Logger.Warn("plan current read failed", map[string]string{
				"error": currentError.Error(),
			})
		}
		return &apiError{Status: http.StatusInternalServerError, Message: "failed to parse plan file"}
	}

	writeJSON(w, http.StatusOK, planCurrentResponse{
		L1: currentWork.L1,
		L2: currentWork.L2,
	})
	return nil
}

func planETag(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("\"%x\"", sum)
}

func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}
