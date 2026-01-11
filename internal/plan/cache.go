package plan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gestalt/internal/logging"
)

type CurrentWork struct {
	L1 string
	L2 string
}

type Cache struct {
	path         string
	logger       *logging.Logger
	mutex        sync.RWMutex
	cachedWork   CurrentWork
	lastModified time.Time
	loaded       bool
}

func NewCache(planPath string, logger *logging.Logger) *Cache {
	pathValue := strings.TrimSpace(planPath)
	if pathValue == "" {
		pathValue = DefaultPath()
	}
	if absolutePath, err := filepath.Abs(pathValue); err == nil {
		pathValue = absolutePath
	}
	return &Cache{
		path:   pathValue,
		logger: logger,
	}
}

func (cache *Cache) Current() (CurrentWork, error) {
	if cache == nil {
		return CurrentWork{}, nil
	}
	pathValue := cache.path
	if pathValue == "" {
		pathValue = DefaultPath()
	}

	info, statError := os.Stat(pathValue)
	if statError != nil {
		if os.IsNotExist(statError) {
			cache.store(CurrentWork{}, time.Time{})
			return CurrentWork{}, nil
		}
		return CurrentWork{}, statError
	}
	modTime := info.ModTime()

	cache.mutex.RLock()
	loaded := cache.loaded
	cachedWork := cache.cachedWork
	cachedModified := cache.lastModified
	cache.mutex.RUnlock()

	if loaded && !modTime.After(cachedModified) {
		return cachedWork, nil
	}

	return cache.Reload()
}

func (cache *Cache) Reload() (CurrentWork, error) {
	if cache == nil {
		return CurrentWork{}, nil
	}
	pathValue := cache.path
	if pathValue == "" {
		pathValue = DefaultPath()
	}

	info, statError := os.Stat(pathValue)
	if statError != nil {
		if os.IsNotExist(statError) {
			cache.store(CurrentWork{}, time.Time{})
			return CurrentWork{}, nil
		}
		return CurrentWork{}, statError
	}
	content, readError := os.ReadFile(pathValue)
	if readError != nil {
		if os.IsNotExist(readError) {
			cache.store(CurrentWork{}, time.Time{})
			return CurrentWork{}, nil
		}
		return CurrentWork{}, readError
	}

	currentWork, summary, parseError := ParseCurrentWork(string(content))
	if parseError != nil {
		return CurrentWork{}, parseError
	}
	cache.store(currentWork, info.ModTime())

	if summary.WipL1Count > 1 {
		cache.logWarn("multiple WIP L1 headings found", map[string]string{
			"count": strconv.Itoa(summary.WipL1Count),
		})
	}
	if summary.WipL2Count > 1 {
		cache.logWarn("multiple WIP L2 headings found", map[string]string{
			"count": strconv.Itoa(summary.WipL2Count),
		})
	}

	return currentWork, nil
}

func (cache *Cache) MatchesPath(pathValue string) bool {
	if cache == nil {
		return false
	}
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return false
	}
	candidate := trimmed
	if absoluteCandidate, err := filepath.Abs(trimmed); err == nil {
		candidate = absoluteCandidate
	}
	cached := cache.path
	if cached == "" {
		return false
	}
	if filepath.Clean(candidate) == filepath.Clean(cached) {
		return true
	}
	return filepath.Base(candidate) == filepath.Base(cached)
}

func (cache *Cache) store(work CurrentWork, modified time.Time) {
	cache.mutex.Lock()
	cache.cachedWork = work
	cache.lastModified = modified
	cache.loaded = true
	cache.mutex.Unlock()
}

func (cache *Cache) logWarn(message string, fields map[string]string) {
	if cache == nil || cache.logger == nil {
		return
	}
	cache.logger.Warn(message, fields)
}
