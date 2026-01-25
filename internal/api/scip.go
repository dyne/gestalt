//go:build !noscip

package api

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/metrics"
	"gestalt/internal/scip"
	"gestalt/internal/watcher"

	"golang.org/x/time/rate"
)

const (
	scipDefaultLimit           = 20
	scipCacheTTL               = 30 * time.Second
	scipCacheMaxEntries        = 256
	scipAutoReindexAge         = 24 * time.Hour
	scipReindexRecentThreshold = 10 * time.Minute
	scipReindexDebounce        = 2 * time.Minute
	scipQueryFindSymbols       = "find_symbols"
	scipQueryGetSymbol         = "get_symbol"
	scipQueryReferences        = "get_references"
	scipQueryFileSymbols       = "file_symbols"
)

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
	indexPath   string
	scipDir     string
	logger      *logging.Logger
	projectRoot string
	indexErr    error

	indexMu sync.RWMutex
	index   *scip.Index

	cache       *queryCache
	rateLimiter *rate.Limiter

	asyncIndexer    *scip.AsyncIndexer
	detectLangs     func(string) ([]string, error)
	findIndexerPath func(string, string) (string, error)
	runIndexer      func(string, string, string) error
	mergeIndexes    func([]string, string) error
	convert         func(string, string) error
	openIndex       func(string) (*scip.Index, error)

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
	Documents   int      `json:"documents"`
	Symbols     int      `json:"symbols"`
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
		cache:             newQueryCache(scipCacheTTL),
		rateLimiter:       rate.NewLimiter(rate.Limit(20), 40),
		detectLangs:       scip.DetectLanguages,
		findIndexerPath:   scip.FindIndexerPath,
		runIndexer:        scip.RunIndexer,
		mergeIndexes:      scip.MergeIndexes,
		convert:           scip.ConvertToSQLite,
		openIndex:         scip.OpenIndex,
		autoReindex:       options.AutoReindex,
		autoReindexMaxAge: options.AutoReindexMaxAge,
		watchDebounce:     options.WatchDebounce,
	}
	handler.enqueueReindex = handler.queueReindex
	handler.asyncIndexer = scip.NewAsyncIndexer(logger, options.SCIPEventBus)
	handler.asyncIndexer.SetOnSuccess(handler.reloadIndex)
	handler.configureIndexer()

	if handler.autoReindexMaxAge <= 0 {
		handler.autoReindexMaxAge = scipAutoReindexAge
	}
	if handler.watchDebounce <= 0 {
		handler.watchDebounce = scipReindexDebounce
	}

	if _, err := os.Stat(indexPath); err == nil {
		index, err := scip.OpenIndex(indexPath)
		if err != nil {
			handler.indexErr = err
		} else {
			handler.index = index
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		handler.indexErr = err
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

// GET /api/scip/symbols?q=FindUser&limit=10
func (h *SCIPHandler) FindSymbols(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

	start := time.Now()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing query"}
	}
	limit, limitErr := parseLimit(r, scipDefaultLimit)
	if limitErr != nil {
		return limitErr
	}

	cacheKey := fmt.Sprintf("find:%s:%d", strings.ToLower(query), limit)
	if cached, ok := h.cache.getSymbols(cacheKey); ok {
		h.recordQuery(scipQueryFindSymbols, start, true)
		writeJSON(w, http.StatusOK, map[string]any{
			"query":   query,
			"symbols": cached,
		})
		return nil
	}

	index, apiErr := h.withIndex()
	if apiErr != nil {
		return apiErr
	}
	symbols, err := index.FindSymbols(query, limit)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}

	h.cache.setSymbols(cacheKey, symbols)
	h.recordQuery(scipQueryFindSymbols, start, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"symbols": symbols,
	})
	return nil
}

// HandleSymbol dispatches /api/scip/symbols/{id} and /api/scip/symbols/{id}/references.
func (h *SCIPHandler) HandleSymbol(w http.ResponseWriter, r *http.Request) *apiError {
	if err := h.allowRequest(); err != nil {
		return err
	}

	suffix := strings.TrimPrefix(r.URL.Path, "/api/scip/symbols/")
	if suffix == "" {
		return &apiError{Status: http.StatusNotFound, Message: "symbol not found"}
	}

	if strings.HasSuffix(suffix, "/references") {
		return h.GetReferences(w, r, strings.TrimSuffix(suffix, "/references"))
	}

	return h.GetSymbol(w, r, suffix)
}

// GET /api/scip/symbols/{id}
func (h *SCIPHandler) GetSymbol(w http.ResponseWriter, r *http.Request, rawSymbol string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	start := time.Now()
	symbolID, err := url.PathUnescape(strings.TrimPrefix(rawSymbol, "/"))
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid symbol id"}
	}

	cacheKey := fmt.Sprintf("symbol:%s", symbolID)
	if cached, ok := h.cache.getSymbol(cacheKey); ok {
		h.recordQuery(scipQueryGetSymbol, start, true)
		writeJSON(w, http.StatusOK, cached)
		return nil
	}

	index, apiErr := h.withIndex()
	if apiErr != nil {
		return apiErr
	}
	symbol, err := index.GetDefinition(symbolID)
	if err != nil {
		var notFound scip.SymbolNotFoundError
		if errors.As(err, &notFound) {
			return &apiError{Status: http.StatusNotFound, Message: "symbol not found"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}

	h.cache.setSymbol(cacheKey, symbol)
	h.recordQuery(scipQueryGetSymbol, start, false)
	writeJSON(w, http.StatusOK, symbol)
	return nil
}

// GET /api/scip/symbols/{id}/references
func (h *SCIPHandler) GetReferences(w http.ResponseWriter, r *http.Request, rawSymbol string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	start := time.Now()
	symbolID, err := url.PathUnescape(strings.TrimPrefix(rawSymbol, "/"))
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid symbol id"}
	}

	cacheKey := fmt.Sprintf("refs:%s", symbolID)
	if cached, ok := h.cache.getOccurrences(cacheKey); ok {
		h.recordQuery(scipQueryReferences, start, true)
		writeJSON(w, http.StatusOK, map[string]any{
			"symbol":     symbolID,
			"references": cached,
		})
		return nil
	}

	index, apiErr := h.withIndex()
	if apiErr != nil {
		return apiErr
	}
	refs, err := index.GetReferences(symbolID)
	if err != nil {
		var notFound scip.SymbolNotFoundError
		if errors.As(err, &notFound) {
			return &apiError{Status: http.StatusNotFound, Message: "symbol not found"}
		}
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}

	h.cache.setOccurrences(cacheKey, refs)
	h.recordQuery(scipQueryReferences, start, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"symbol":     symbolID,
		"references": refs,
	})
	return nil
}

// GET /api/scip/files/{path}
func (h *SCIPHandler) GetFileSymbols(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

	start := time.Now()
	suffix := strings.TrimPrefix(r.URL.Path, "/api/scip/files/")
	if suffix == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing file path"}
	}
	filePath, err := url.PathUnescape(suffix)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid file path"}
	}

	cacheKey := fmt.Sprintf("file:%s", filePath)
	if cached, ok := h.cache.getSymbols(cacheKey); ok {
		h.recordQuery(scipQueryFileSymbols, start, true)
		writeJSON(w, http.StatusOK, map[string]any{
			"file":    filePath,
			"symbols": cached,
		})
		return nil
	}

	index, apiErr := h.withIndex()
	if apiErr != nil {
		return apiErr
	}
	symbols, err := index.GetSymbolsInFile(filePath)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}
	h.cache.setSymbols(cacheKey, symbols)
	h.recordQuery(scipQueryFileSymbols, start, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"file":    filePath,
		"symbols": symbols,
	})
	return nil
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

// GET /api/scip/status
func (h *SCIPHandler) Status(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

	status, err := h.buildStatus()
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}
	writeJSON(w, http.StatusOK, status)
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
		ConvertToSQLite: h.convert,
	})
}

func (h *SCIPHandler) reloadIndex(status scip.IndexStatus) error {
	openIndex := h.openIndex
	if openIndex == nil {
		openIndex = scip.OpenIndex
	}
	index, err := openIndex(status.IndexPath)
	if err != nil {
		h.logWarn("scip index reload failed", err)
		return err
	}

	h.indexMu.Lock()
	oldIndex := h.index
	h.index = index
	h.indexMu.Unlock()
	if oldIndex != nil {
		_ = oldIndex.Close()
	}
	if h.cache != nil {
		h.cache.clear()
	}
	return nil
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

func (h *SCIPHandler) withIndex() (*scip.Index, *apiError) {
	h.indexMu.RLock()
	index := h.index
	h.indexMu.RUnlock()
	if index == nil {
		return nil, &apiError{Status: http.StatusServiceUnavailable, Message: "scip index unavailable"}
	}
	return index, nil
}

func (h *SCIPHandler) recordQuery(queryType string, start time.Time, cacheHit bool) {
	metrics.Default.RecordSCIPQuery(queryType, time.Since(start), cacheHit)
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

	h.indexMu.RLock()
	index := h.index
	h.indexMu.RUnlock()
	if index != nil {
		stats, err := index.GetStats()
		if err != nil {
			h.logWarn("scip stats unavailable", err)
		} else {
			status.Documents = stats.Documents
			status.Symbols = stats.Symbols
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
	fields := map[string]string{}
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
		"documents": strconv.Itoa(status.Documents),
		"symbols":   strconv.Itoa(status.Symbols),
	}
	if status.CreatedAt != "" {
		fields["created_at"] = status.CreatedAt
	}
	if status.AgeHours > 0 {
		fields["age_hours"] = strconv.Itoa(status.AgeHours)
	}
	if len(status.Languages) > 0 {
		fields["languages"] = strings.Join(status.Languages, ",")
	}
	if status.InProgress {
		fields["in_progress"] = "true"
	}
	if strings.TrimSpace(status.Error) != "" {
		fields["error"] = status.Error
	}
	h.logger.Info("scip index loaded", fields)
	if !status.Fresh {
		h.logger.Warn("scip index is stale, consider re-indexing", fields)
	}
}

func parseLimit(r *http.Request, fallback int) (int, *apiError) {
	if fallback <= 0 {
		fallback = scipDefaultLimit
	}
	limitValue := r.URL.Query().Get("limit")
	if limitValue == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(limitValue)
	if err != nil || parsed <= 0 {
		return 0, &apiError{Status: http.StatusBadRequest, Message: "invalid limit"}
	}
	return parsed, nil
}

type queryCache struct {
	ttl        time.Duration
	maxEntries int
	mu         sync.Mutex
	entries    map[string]*list.Element
	order      *list.List
}

type cacheEntry struct {
	key       string
	expiresAt time.Time
	payload   any
}

func newQueryCache(ttl time.Duration) *queryCache {
	if ttl <= 0 {
		ttl = scipCacheTTL
	}
	return &queryCache{
		ttl:        ttl,
		maxEntries: scipCacheMaxEntries,
		entries:    make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (cache *queryCache) clear() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.entries = make(map[string]*list.Element)
	if cache.order == nil {
		cache.order = list.New()
		return
	}
	cache.order.Init()
}

func (cache *queryCache) getSymbols(key string) ([]scip.Symbol, bool) {
	entry, ok := cache.get(key)
	if !ok {
		return nil, false
	}
	symbols, ok := entry.([]scip.Symbol)
	if !ok {
		return nil, false
	}
	return cloneSymbols(symbols), true
}

func (cache *queryCache) setSymbols(key string, symbols []scip.Symbol) {
	cache.set(key, cloneSymbols(symbols))
}

func (cache *queryCache) getSymbol(key string) (*scip.Symbol, bool) {
	entry, ok := cache.get(key)
	if !ok {
		return nil, false
	}
	symbol, ok := entry.(scip.Symbol)
	if !ok {
		return nil, false
	}
	cloned := cloneSymbol(symbol)
	return &cloned, true
}

func (cache *queryCache) setSymbol(key string, symbol *scip.Symbol) {
	if symbol == nil {
		return
	}
	clone := cloneSymbol(*symbol)
	cache.set(key, clone)
}

func (cache *queryCache) getOccurrences(key string) ([]scip.Occurrence, bool) {
	entry, ok := cache.get(key)
	if !ok {
		return nil, false
	}
	occurrences, ok := entry.([]scip.Occurrence)
	if !ok {
		return nil, false
	}
	return cloneOccurrences(occurrences), true
}

func (cache *queryCache) setOccurrences(key string, occurrences []scip.Occurrence) {
	cache.set(key, cloneOccurrences(occurrences))
}

func (cache *queryCache) get(key string) (any, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	element, ok := cache.entries[key]
	if !ok {
		return nil, false
	}
	entry := element.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		cache.removeElement(element)
		return nil, false
	}
	cache.order.MoveToFront(element)
	return entry.payload, true
}

func (cache *queryCache) set(key string, payload any) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if element, ok := cache.entries[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.payload = payload
		entry.expiresAt = time.Now().Add(cache.ttl)
		cache.order.MoveToFront(element)
		return
	}

	element := cache.order.PushFront(&cacheEntry{
		key:       key,
		expiresAt: time.Now().Add(cache.ttl),
		payload:   payload,
	})
	cache.entries[key] = element

	if cache.maxEntries <= 0 {
		cache.maxEntries = scipCacheMaxEntries
	}
	if cache.order.Len() > cache.maxEntries {
		cache.removeOldest()
	}
}

func (cache *queryCache) removeOldest() {
	element := cache.order.Back()
	if element == nil {
		return
	}
	cache.removeElement(element)
}

func (cache *queryCache) removeElement(element *list.Element) {
	cache.order.Remove(element)
	entry := element.Value.(*cacheEntry)
	delete(cache.entries, entry.key)
}

func cloneSymbols(symbols []scip.Symbol) []scip.Symbol {
	if symbols == nil {
		return nil
	}
	cloned := make([]scip.Symbol, len(symbols))
	for index, symbol := range symbols {
		cloned[index] = cloneSymbol(symbol)
	}
	return cloned
}

func cloneSymbol(symbol scip.Symbol) scip.Symbol {
	cloned := symbol
	if symbol.Documentation != nil {
		cloned.Documentation = append([]string(nil), symbol.Documentation...)
	}
	return cloned
}

func cloneOccurrences(occurrences []scip.Occurrence) []scip.Occurrence {
	if occurrences == nil {
		return nil
	}
	cloned := make([]scip.Occurrence, len(occurrences))
	copy(cloned, occurrences)
	return cloned
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
