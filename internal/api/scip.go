//go:build !noscip

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/scip"
	"gestalt/internal/watcher"

	"golang.org/x/time/rate"
)

const (
	scipAutoReindexAge         = 24 * time.Hour
	scipReindexRecentThreshold = 10 * time.Minute
	scipReindexDebounce        = 2 * time.Minute
)

type SCIPHandlerOptions struct {
	ProjectRoot        string
	AutoReindex        bool
	AutoReindexMaxAge  time.Duration
	WatchDebounce      time.Duration
	EventBus           *event.Bus[watcher.Event]
	AutoReindexOnStart bool
}

type SCIPHandler struct {
	indexPath   string
	scipDir     string
	logger      *logging.Logger
	projectRoot string

	rateLimiter *rate.Limiter

	asyncIndexer    *scip.AsyncIndexer
	detectLangs     func(string) ([]string, error)
	findIndexerPath func(string, string) (string, error)
	runIndexer      func(string, string, string) error
	mergeIndexes    func([]string, string) error

	autoReindex       bool
	autoReindexMaxAge time.Duration
	watchDebounce     time.Duration
	watchMu           sync.Mutex
	watchTimer        *time.Timer
	enqueueReindex    func(string)
}

type scipStatusResponse struct {
	Indexed     bool     `json:"indexed"`
	Fresh       bool     `json:"fresh"`
	InProgress  bool     `json:"in_progress"`
	StartedAt   string   `json:"started_at,omitempty"`
	CompletedAt string   `json:"completed_at,omitempty"`
	Duration    string   `json:"duration,omitempty"`
	Error       string   `json:"error,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	AgeHours    int      `json:"age_hours"`
	Languages   []string `json:"languages,omitempty"`
}

type reindexResult struct {
	Started  bool
	Conflict bool
	Recent   bool
	Age      time.Duration
}

func NewSCIPHandler(indexPath string, logger *logging.Logger, options SCIPHandlerOptions) (*SCIPHandler, error) {
	if strings.TrimSpace(indexPath) == "" {
		return nil, fmt.Errorf("scip index path is required")
	}

	projectRoot := strings.TrimSpace(options.ProjectRoot)
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}
	if absRoot, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = absRoot
	}

	handler := &SCIPHandler{
		indexPath:         indexPath,
		scipDir:           filepath.Dir(indexPath),
		logger:            logger,
		projectRoot:       projectRoot,
		rateLimiter:       rate.NewLimiter(rate.Limit(20), 40),
		detectLangs:       scip.DetectLanguages,
		findIndexerPath:   scip.FindIndexerPath,
		runIndexer:        scip.RunIndexer,
		mergeIndexes:      scip.MergeIndexes,
		autoReindex:       options.AutoReindex,
		autoReindexMaxAge: options.AutoReindexMaxAge,
		watchDebounce:     options.WatchDebounce,
	}
	handler.enqueueReindex = handler.queueReindex
	handler.asyncIndexer = scip.NewAsyncIndexer(logger)
	handler.configureIndexer()

	if handler.autoReindexMaxAge <= 0 {
		handler.autoReindexMaxAge = scipAutoReindexAge
	}
	if handler.watchDebounce <= 0 {
		handler.watchDebounce = scipReindexDebounce
	}

	handler.logIndexStatus()

	if handler.autoReindex && options.AutoReindexOnStart {
		handler.maybeAutoReindex()
	}
	if handler.autoReindex && options.EventBus != nil {
		handler.watchFileEvents(options.EventBus)
	}

	return handler, nil
}

// POST /api/scip/index
func (h *SCIPHandler) ReIndex(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, http.MethodPost)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

	var req struct {
		Path      string   `json:"path"`
		Force     bool     `json:"force"`
		Languages []string `json:"languages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "path is required"}
	}

	result, err := h.triggerReindex(path, req.Languages, req.Force)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}

	if result.Conflict {
		return &apiError{Status: http.StatusConflict, Message: "indexing already in progress"}
	}
	if result.Recent {
		h.logWarn("scip reindex skipped (recent index)", nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "recent",
			"message": fmt.Sprintf("Index was created %s ago. Use force to reindex.", result.Age.Round(time.Second)),
		})
		return nil
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "indexing started"})
	return nil
}

// POST /api/scip/reindex
func (h *SCIPHandler) Reindex(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodPost {
		return methodNotAllowed(w, http.MethodPost)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

	var req struct {
		Path      string   `json:"path"`
		Languages []string `json:"languages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = h.projectRoot
	}
	if strings.TrimSpace(path) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "path is required"}
	}

	result, err := h.triggerReindex(path, req.Languages, true)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}
	if result.Conflict {
		return &apiError{Status: http.StatusConflict, Message: "indexing already in progress"}
	}
	if result.Recent {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "recent",
			"message": fmt.Sprintf("Index was created %s ago.", result.Age.Round(time.Second)),
		})
		return nil
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "indexing started"})
	return nil
}

func (h *SCIPHandler) triggerReindex(path string, languages []string, force bool) (reindexResult, error) {
	root := strings.TrimSpace(path)
	if root == "" {
		root = h.projectRoot
	}
	if strings.TrimSpace(root) == "" {
		return reindexResult{}, fmt.Errorf("path is required")
	}
	if h.asyncIndexer == nil {
		return reindexResult{}, fmt.Errorf("scip indexer unavailable")
	}

	if status := h.asyncIndexer.Status(); status.InProgress {
		return reindexResult{Conflict: true}, nil
	}

	if !force {
		recent, age, err := h.recentIndexAge(scipReindexRecentThreshold)
		if err != nil {
			h.logWarn("scip metadata read failed", err)
		} else if recent {
			return reindexResult{Recent: true, Age: age}, nil
		}
	}

	h.configureIndexer()
	started := h.asyncIndexer.StartAsync(scip.IndexRequest{
		ProjectRoot: root,
		ScipDir:     h.scipDir,
		IndexPath:   h.indexPath,
		Languages:   languages,
		Merge:       true,
	})
	if !started {
		return reindexResult{Conflict: true}, nil
	}
	return reindexResult{Started: true}, nil
}

func (h *SCIPHandler) configureIndexer() {
	if h.asyncIndexer == nil {
		return
	}
	h.asyncIndexer.SetDependencies(scip.AsyncIndexerDeps{
		DetectLanguages: h.detectLangs,
		FindIndexerPath: h.findIndexerPath,
		RunIndexer:      h.runIndexer,
		MergeIndexes:    h.mergeIndexes,
	})
}

func (h *SCIPHandler) StartAutoIndexing() bool {
	if h.asyncIndexer == nil {
		return false
	}
	status, err := h.buildStatus()
	if err != nil {
		h.logWarn("scip status check failed", err)
	}
	if status.InProgress {
		return false
	}
	if status.Indexed && status.Fresh {
		return false
	}
	result, err := h.triggerReindex(h.projectRoot, nil, true)
	if err != nil {
		h.logWarn("scip auto indexing failed", err)
		return false
	}
	return result.Started
}

func (h *SCIPHandler) recentIndexAge(threshold time.Duration) (bool, time.Duration, error) {
	if _, err := os.Stat(h.indexPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	meta, err := scip.LoadMetadata(h.indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if meta.CreatedAt.IsZero() {
		return false, 0, nil
	}
	age := time.Since(meta.CreatedAt)
	if age < 0 {
		age = 0
	}
	return age < threshold, age, nil
}

func (h *SCIPHandler) buildStatus() (scipStatusResponse, error) {
	indexed := fileExists(h.indexPath)
	status := scipStatusResponse{
		Indexed: indexed,
	}
	indexerStatus := scip.IndexStatus{}
	if h.asyncIndexer != nil {
		indexerStatus = h.asyncIndexer.Status()
	}
	status.InProgress = indexerStatus.InProgress
	if !indexerStatus.StartedAt.IsZero() {
		status.StartedAt = indexerStatus.StartedAt.UTC().Format(time.RFC3339)
	}
	if !indexerStatus.CompletedAt.IsZero() {
		status.CompletedAt = indexerStatus.CompletedAt.UTC().Format(time.RFC3339)
	}
	duration := indexerStatus.Duration
	if indexerStatus.InProgress && duration <= 0 && !indexerStatus.StartedAt.IsZero() {
		duration = time.Since(indexerStatus.StartedAt)
	}
	if duration < 0 {
		duration = 0
	}
	if duration > 0 {
		status.Duration = duration.Round(time.Second).String()
	}
	if strings.TrimSpace(indexerStatus.Error) != "" {
		status.Error = strings.TrimSpace(indexerStatus.Error)
	}
	if len(indexerStatus.Languages) > 0 {
		status.Languages = append([]string(nil), indexerStatus.Languages...)
	}

	var createdAt time.Time
	var metaLanguages []string
	meta, metaErr := scip.LoadMetadata(h.indexPath)
	if metaErr == nil && !meta.CreatedAt.IsZero() {
		createdAt = meta.CreatedAt
		metaLanguages = append([]string(nil), meta.Languages...)
		if fresh, err := scip.IsFresh(meta); err == nil {
			status.Fresh = fresh
		}
	} else if indexed {
		if info, err := os.Stat(h.indexPath); err == nil {
			createdAt = info.ModTime().UTC()
		}
	}

	if !createdAt.IsZero() {
		status.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		age := time.Since(createdAt)
		if age > 0 {
			status.AgeHours = int(age.Hours())
		}
	}

	if len(status.Languages) == 0 && len(metaLanguages) > 0 {
		status.Languages = metaLanguages
	}

	if !indexed {
		status.Fresh = false
	}

	return status, nil
}

func (h *SCIPHandler) maybeAutoReindex() {
	if !h.autoReindex || strings.TrimSpace(h.projectRoot) == "" {
		return
	}
	status, err := h.buildStatus()
	if err != nil {
		h.logWarn("scip status check failed", err)
		return
	}
	if status.InProgress {
		return
	}
	if !status.Indexed {
		return
	}
	stale := !status.Fresh
	if h.autoReindexMaxAge > 0 && status.AgeHours > 0 {
		if time.Duration(status.AgeHours)*time.Hour >= h.autoReindexMaxAge {
			stale = true
		}
	}
	if !stale {
		return
	}
	h.enqueueReindex(h.projectRoot)
}

func (h *SCIPHandler) watchFileEvents(bus *event.Bus[watcher.Event]) {
	events, _ := bus.SubscribeFiltered(func(evt watcher.Event) bool {
		return evt.Type == watcher.EventTypeFileChanged
	})
	go func() {
		for evt := range events {
			if !h.shouldReindexForPath(evt.Path) {
				continue
			}
			h.scheduleReindex(h.projectRoot)
		}
	}()
}

func (h *SCIPHandler) scheduleReindex(path string) {
	h.watchMu.Lock()
	defer h.watchMu.Unlock()
	if h.watchTimer == nil {
		h.watchTimer = time.AfterFunc(h.watchDebounce, func() {
			h.enqueueReindex(path)
		})
		return
	}
	h.watchTimer.Reset(h.watchDebounce)
}

func (h *SCIPHandler) queueReindex(path string) {
	if _, err := h.triggerReindex(path, nil, true); err != nil {
		h.logWarn("scip reindex failed", err)
	}
}

func (h *SCIPHandler) shouldReindexForPath(path string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(h.projectRoot) == "" {
		return false
	}
	absPath := path
	if !filepath.IsAbs(absPath) {
		if abs, err := filepath.Abs(absPath); err == nil {
			absPath = abs
		}
	}
	root := h.projectRoot
	if !strings.HasPrefix(absPath, root) {
		return false
	}
	if len(absPath) > len(root) && absPath[len(root)] != os.PathSeparator {
		return false
	}
	return scip.IsSupportedSourcePath(absPath)
}

func (h *SCIPHandler) allowRequest() *apiError {
	if h.rateLimiter == nil || h.rateLimiter.Allow() {
		return nil
	}
	return &apiError{Status: http.StatusTooManyRequests, Message: "rate limit exceeded"}
}

func (h *SCIPHandler) logWarn(message string, err error) {
	if h.logger == nil {
		return
	}
	fields := map[string]string{
		"gestalt.category": "scip",
		"gestalt.source":   "backend",
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	h.logger.Warn(message, fields)
}

func (h *SCIPHandler) logIndexStatus() {
	if h.logger == nil {
		return
	}
	status, err := h.buildStatus()
	if err != nil {
		h.logger.Warn("scip status unavailable", map[string]string{
			"error": err.Error(),
		})
		return
	}
	if !status.Indexed {
		h.logger.Warn("scip index missing", nil)
		return
	}
	fields := map[string]string{
		"index_path": h.indexPath,
	}
	if status.CreatedAt != "" {
		fields["created_at"] = status.CreatedAt
	}
	if status.Fresh {
		h.logger.Info("scip index loaded", fields)
		return
	}
	h.logger.Warn("scip index is stale, consider re-indexing", fields)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
