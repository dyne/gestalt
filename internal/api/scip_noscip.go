//go:build noscip

package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/watcher"
)

const scipDisabledMessage = "scip disabled at build time"

type SCIPHandlerOptions struct {
	ProjectRoot        string
	AutoReindex        bool
	AutoReindexMaxAge  time.Duration
	WatchDebounce      time.Duration
	EventBus           *event.Bus[watcher.Event]
	SCIPEventBus       *event.Bus[event.SCIPEvent]
	AutoReindexOnStart bool
}

type SCIPHandler struct {
	logger *logging.Logger
}

func NewSCIPHandler(indexPath string, logger *logging.Logger, _ SCIPHandlerOptions) (*SCIPHandler, error) {
	if strings.TrimSpace(indexPath) == "" {
		return nil, fmt.Errorf("scip index path is required")
	}
	if logger != nil {
		logger.Warn(scipDisabledMessage, map[string]string{
			"index_path": indexPath,
		})
	}
	return &SCIPHandler{logger: logger}, nil
}

func (h *SCIPHandler) Status(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) FindSymbols(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) HandleSymbol(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) GetFileSymbols(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) ReIndex(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) Reindex(http.ResponseWriter, *http.Request) *apiError {
	return scipDisabledError()
}

func (h *SCIPHandler) StartAutoIndexing() bool {
	return false
}

func scipDisabledError() *apiError {
	return &apiError{Status: http.StatusNotImplemented, Message: scipDisabledMessage, Code: "scip_disabled"}
}
