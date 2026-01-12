package api

import (
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

	"gestalt/internal/logging"
	"gestalt/internal/scip"

	"golang.org/x/time/rate"
)

const (
	scipDefaultLimit = 20
	scipCacheTTL     = 30 * time.Second
)

type SCIPHandler struct {
	indexPath string
	logger    *logging.Logger

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
}

func NewSCIPHandler(indexPath string, logger *logging.Logger) (*SCIPHandler, error) {
	if strings.TrimSpace(indexPath) == "" {
		return nil, fmt.Errorf("scip index path is required")
	}
	if _, err := os.Stat(indexPath); err != nil {
		return nil, fmt.Errorf("scip index not available: %w", err)
	}

	index, err := scip.OpenIndex(indexPath)
	if err != nil {
		return nil, err
	}

	return &SCIPHandler{
		indexPath:   indexPath,
		logger:      logger,
		index:       index,
		cache:       newQueryCache(scipCacheTTL),
		rateLimiter: rate.NewLimiter(rate.Limit(20), 40),
		detectLangs: scip.DetectLanguages,
		runIndexer:  scip.RunIndexer,
		convert:     scip.ConvertToSQLite,
		openIndex:   scip.OpenIndex,
	}, nil
}

// GET /api/scip/symbols?q=FindUser&limit=10
func (h *SCIPHandler) FindSymbols(w http.ResponseWriter, r *http.Request) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	if err := h.allowRequest(); err != nil {
		return err
	}

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
	symbolID, err := url.PathUnescape(strings.TrimPrefix(rawSymbol, "/"))
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid symbol id"}
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

	writeJSON(w, http.StatusOK, symbol)
	return nil
}

// GET /api/scip/symbols/{id}/references
func (h *SCIPHandler) GetReferences(w http.ResponseWriter, r *http.Request, rawSymbol string) *apiError {
	if r.Method != http.MethodGet {
		return methodNotAllowed(w, http.MethodGet)
	}
	symbolID, err := url.PathUnescape(strings.TrimPrefix(rawSymbol, "/"))
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid symbol id"}
	}

	cacheKey := fmt.Sprintf("refs:%s", symbolID)
	if cached, ok := h.cache.getOccurrences(cacheKey); ok {
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

	suffix := strings.TrimPrefix(r.URL.Path, "/api/scip/files/")
	if suffix == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "missing file path"}
	}
	filePath, err := url.PathUnescape(suffix)
	if err != nil {
		return &apiError{Status: http.StatusBadRequest, Message: "invalid file path"}
	}

	index, apiErr := h.withIndex()
	if apiErr != nil {
		return apiErr
	}
	symbols, err := index.GetSymbolsInFile(filePath)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Message: err.Error()}
	}
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
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	expiresAt time.Time
	payload   any
}

func newQueryCache(ttl time.Duration) *queryCache {
	if ttl <= 0 {
		ttl = scipCacheTTL
	}
	return &queryCache{
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
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

	entry, ok := cache.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(cache.entries, key)
		return nil, false
	}
	return entry.payload, true
}

func (cache *queryCache) set(key string, payload any) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.entries[key] = cacheEntry{
		expiresAt: time.Now().Add(cache.ttl),
		payload:   payload,
	}
}

func cloneSymbols(symbols []scip.Symbol) []scip.Symbol {
	if symbols == nil {
		return nil
	}
	cloned := make([]scip.Symbol, len(symbols))
	for index, symbol := range symbols {
		cloned[index] = symbol
		if symbol.Documentation != nil {
			cloned[index].Documentation = append([]string(nil), symbol.Documentation...)
		}
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
