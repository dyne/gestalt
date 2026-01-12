package api

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
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
	scipDefaultLimit     = 20
	scipCacheTTL         = 30 * time.Second
	scipCacheMaxEntries  = 256
	scipAutoReindexAge   = 24 * time.Hour
	scipReindexDebounce  = 2 * time.Minute
	scipQueryFindSymbols = "find_symbols"
	scipQueryGetSymbol   = "get_symbol"
	scipQueryReferences  = "get_references"
	scipQueryFileSymbols = "file_symbols"
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
	logger      *logging.Logger
	projectRoot string
	indexErr    error

	indexMu sync.RWMutex
	index   *scip.Index

	cache       *queryCache
	rateLimiter *rate.Limiter

	reindexMu   sync.Mutex
	reindexing  bool
	detectLangs func(string) ([]string, error)
	runIndexer  func(string, string, string) error
	convert     func(string, string) error
	openIndex   func(string) (*scip.Index, error)

	autoReindex       bool
	autoReindexMaxAge time.Duration
	watchDebounce     time.Duration
	watchMu           sync.Mutex
	watchTimer        *time.Timer
	enqueueReindex    func(string)
}

type scipStatusResponse struct {
	Indexed   bool   `json:"indexed"`
	Fresh     bool   `json:"fresh"`
	CreatedAt string `json:"created_at,omitempty"`
	Documents int    `json:"documents"`
	Symbols   int    `json:"symbols"`
	AgeHours  int    `json:"age_hours"`
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
		logger:            logger,
		projectRoot:       projectRoot,
		cache:             newQueryCache(scipCacheTTL),
		rateLimiter:       rate.NewLimiter(rate.Limit(20), 40),
		detectLangs:       scip.DetectLanguages,
		runIndexer:        scip.RunIndexer,
		convert:           scip.ConvertToSQLite,
		openIndex:         scip.OpenIndex,
		autoReindex:       options.AutoReindex,
		autoReindexMaxAge: options.AutoReindexMaxAge,
		watchDebounce:     options.WatchDebounce,
	}
	handler.enqueueReindex = handler.queueReindex

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
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid request body"}
	}
	if strings.TrimSpace(req.Path) == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "path is required"}
	}

	if !h.beginReindex() {
		return &apiError{Status: http.StatusConflict, Message: "indexing already in progress"}
	}

	go h.runReindex(strings.TrimSpace(req.Path))

	writeJSON(w, http.StatusOK, map[string]string{"status": "indexing started"})
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

func (h *SCIPHandler) runReindex(path string) {
	defer h.endReindex()

	langs, err := h.detectLangs(path)
	if err != nil {
		h.logWarn("scip language detection failed", err)
		return
	}
	if len(langs) == 0 {
		h.logWarn("scip reindex skipped (no languages detected)", nil)
		return
	}

	dbDir := filepath.Dir(h.indexPath)
	scipPath := filepath.Join(dbDir, "index.scip")
	for _, lang := range langs {
		if err := h.runIndexer(lang, path, scipPath); err != nil {
			h.logWarn(fmt.Sprintf("scip indexer failed (%s)", lang), err)
			return
		}
	}
	if err := h.convert(scipPath, h.indexPath); err != nil {
		h.logWarn("scip index conversion failed", err)
		return
	}
	meta, err := scip.BuildMetadata(path, langs)
	if err != nil {
		h.logWarn("scip metadata build failed", err)
	} else if err := scip.SaveMetadata(h.indexPath, meta); err != nil {
		h.logWarn("scip metadata write failed", err)
	}
	index, err := h.openIndex(h.indexPath)
	if err != nil {
		h.logWarn("scip index reload failed", err)
		return
	}

	h.indexMu.Lock()
	oldIndex := h.index
	h.index = index
	h.indexMu.Unlock()
	if oldIndex != nil {
		_ = oldIndex.Close()
	}
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

func (h *SCIPHandler) buildStatus() (scipStatusResponse, error) {
	indexed := fileExists(h.indexPath)
	status := scipStatusResponse{
		Indexed: indexed,
	}

	var createdAt time.Time
	meta, metaErr := scip.LoadMetadata(h.indexPath)
	if metaErr == nil && !meta.CreatedAt.IsZero() {
		createdAt = meta.CreatedAt
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
			return status, err
		}
		status.Documents = stats.Documents
		status.Symbols = stats.Symbols
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
	if !h.beginReindex() {
		return
	}
	go h.runReindex(path)
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

func (h *SCIPHandler) beginReindex() bool {
	h.reindexMu.Lock()
	defer h.reindexMu.Unlock()
	if h.reindexing {
		return false
	}
	h.reindexing = true
	return true
}

func (h *SCIPHandler) endReindex() {
	h.reindexMu.Lock()
	h.reindexing = false
	h.reindexMu.Unlock()
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
