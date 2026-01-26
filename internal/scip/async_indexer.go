//go:build !noscip

package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	eventtypes "gestalt/internal/event"
	"gestalt/internal/logging"
)

const mergedScipName = "index.scip"

// IndexStatus captures the most recent indexing lifecycle state.
type IndexStatus struct {
	InProgress  bool
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Error       string
	Languages   []string
	ProjectRoot string
	MergedScip  string
	IndexPath   string
	UpdatedAt   time.Time
}

// IndexRequest describes a single indexing run.
type IndexRequest struct {
	ProjectRoot string
	ScipDir     string
	IndexPath   string
	Languages   []string
	Merge       bool
}

// AsyncIndexer runs SCIP indexing in the background and publishes lifecycle events.
type AsyncIndexer struct {
	mu      sync.RWMutex
	status  IndexStatus
	running bool

	eventBus *eventtypes.Bus[eventtypes.SCIPEvent]
	logger   *logging.Logger

	detectLanguages func(string) ([]string, error)
	findIndexerPath func(string, string) (string, error)
	runIndexer      func(string, string, string) error
	mergeIndexes    func([]string, string) error
	convertToSQLite func(string, string) error
	buildMetadata   func(string, []string) (IndexMetadata, error)
	saveMetadata    func(string, IndexMetadata) error
	onSuccess       func(IndexStatus) error
	now             func() time.Time
}

// AsyncIndexerDeps allows overriding indexing dependencies for testing and integration.
type AsyncIndexerDeps struct {
	DetectLanguages func(string) ([]string, error)
	FindIndexerPath func(string, string) (string, error)
	RunIndexer      func(string, string, string) error
	MergeIndexes    func([]string, string) error
	ConvertToSQLite func(string, string) error
	BuildMetadata   func(string, []string) (IndexMetadata, error)
	SaveMetadata    func(string, IndexMetadata) error
	Now             func() time.Time
}

// NewAsyncIndexer constructs an AsyncIndexer with production defaults.
func NewAsyncIndexer(logger *logging.Logger, bus *eventtypes.Bus[eventtypes.SCIPEvent]) *AsyncIndexer {
	return &AsyncIndexer{
		eventBus:        bus,
		logger:          logger,
		detectLanguages: DetectLanguages,
		findIndexerPath: FindIndexerPath,
		runIndexer:      RunIndexer,
		mergeIndexes:    MergeIndexes,
		convertToSQLite: ConvertToSQLite,
		buildMetadata:   BuildMetadata,
		saveMetadata:    SaveMetadata,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// SetDependencies overrides the default indexing dependencies.
func (idx *AsyncIndexer) SetDependencies(deps AsyncIndexerDeps) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if deps.DetectLanguages != nil {
		idx.detectLanguages = deps.DetectLanguages
	}
	if deps.FindIndexerPath != nil {
		idx.findIndexerPath = deps.FindIndexerPath
	}
	if deps.RunIndexer != nil {
		idx.runIndexer = deps.RunIndexer
	}
	if deps.MergeIndexes != nil {
		idx.mergeIndexes = deps.MergeIndexes
	}
	if deps.ConvertToSQLite != nil {
		idx.convertToSQLite = deps.ConvertToSQLite
	}
	if deps.BuildMetadata != nil {
		idx.buildMetadata = deps.BuildMetadata
	}
	if deps.SaveMetadata != nil {
		idx.saveMetadata = deps.SaveMetadata
	}
	if deps.Now != nil {
		idx.now = deps.Now
	}
}

// SetOnSuccess registers a callback that runs after a successful index build.
func (idx *AsyncIndexer) SetOnSuccess(callback func(IndexStatus) error) {
	idx.mu.Lock()
	idx.onSuccess = callback
	idx.mu.Unlock()
}

// Status returns a snapshot of the current indexing status.
func (idx *AsyncIndexer) Status() IndexStatus {
	idx.mu.RLock()
	status := idx.status
	idx.mu.RUnlock()
	status.Languages = append([]string(nil), status.Languages...)
	return status
}

// StartAsync begins indexing in a background goroutine. It returns false when a run is already in progress.
func (idx *AsyncIndexer) StartAsync(req IndexRequest) bool {
	started, startStatus := idx.begin(req)
	if !started {
		return false
	}
	idx.publish(startStatus, "start", "", "scip indexing started")
	go idx.run(req, startStatus.StartedAt)
	return true
}

func (idx *AsyncIndexer) begin(req IndexRequest) (bool, IndexStatus) {
	projectRoot := strings.TrimSpace(req.ProjectRoot)
	indexPath := strings.TrimSpace(req.IndexPath)
	if indexPath == "" {
		indexPath = filepath.Join(".gestalt", "scip", "index.db")
	}
	scipDir := strings.TrimSpace(req.ScipDir)
	if scipDir == "" && indexPath != "" {
		scipDir = filepath.Dir(indexPath)
	}
	if scipDir == "" {
		scipDir = filepath.Join(".gestalt", "scip")
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.running {
		return false, idx.status
	}
	startedAt := idx.now()
	idx.running = true
	idx.status = IndexStatus{
		InProgress:  true,
		StartedAt:   startedAt,
		Languages:   normalizeLanguages(req.Languages),
		ProjectRoot: projectRoot,
		MergedScip:  filepath.Join(scipDir, mergedScipName),
		IndexPath:   indexPath,
		UpdatedAt:   startedAt,
	}
	return true, idx.status
}

func (idx *AsyncIndexer) run(req IndexRequest, startedAt time.Time) {
	status, err := idx.runIndexing(req, startedAt)
	idx.finish(status, err)
}

func (idx *AsyncIndexer) runIndexing(req IndexRequest, startedAt time.Time) (IndexStatus, error) {
	projectRoot := strings.TrimSpace(req.ProjectRoot)
	if projectRoot == "" {
		return idx.Status(), fmt.Errorf("project root is required")
	}

	indexPath := strings.TrimSpace(req.IndexPath)
	if indexPath == "" {
		indexPath = filepath.Join(".gestalt", "scip", "index.db")
	}
	scipDir := strings.TrimSpace(req.ScipDir)
	if scipDir == "" {
		scipDir = filepath.Dir(indexPath)
	}
	if err := os.MkdirAll(scipDir, 0o755); err != nil {
		return idx.Status(), fmt.Errorf("create scip dir: %w", err)
	}

	detected, err := idx.detectLanguages(projectRoot)
	if err != nil {
		return idx.Status(), err
	}
	requestedLanguages := normalizeLanguages(req.Languages)
	hasLanguageFilter := len(requestedLanguages) > 0
	languages := idx.selectLanguages(detected, requestedLanguages)
	if len(languages) > 0 {
		idx.publish(idx.Status(), "progress", "", "detected languages: "+strings.Join(languages, ", "))
	} else {
		idx.publish(idx.Status(), "progress", "", "no languages detected; using existing indexes")
	}

	for _, language := range languages {
		output := filepath.Join(scipDir, indexFileName(language))
		path, lookupErr := idx.findIndexerPath(language, projectRoot)
		if lookupErr != nil {
			idx.publish(idx.Status(), "progress", language, "indexer unavailable: "+lookupErr.Error())
			continue
		}
		if strings.TrimSpace(path) == "" {
			idx.publish(idx.Status(), "progress", language, "indexer missing; skipping")
			continue
		}
		idx.publish(idx.Status(), "progress", language, "indexing")
		if err := idx.runIndexer(language, projectRoot, output); err != nil {
			idx.publish(idx.Status(), "progress", language, "indexer failed: "+err.Error())
			continue
		}
	}

	allowedInputs := []string(nil)
	if hasLanguageFilter {
		allowedInputs = requestedLanguages
	}
	inputs, err := collectScipInputs(scipDir, allowedInputs)
	if err != nil {
		return idx.Status(), err
	}
	mergedPath := filepath.Join(scipDir, mergedScipName)
	if len(inputs) == 0 && !hasLanguageFilter && fileExists(mergedPath) {
		inputs = []string{mergedPath}
	}
	if len(inputs) == 0 {
		return idx.Status(), fmt.Errorf("no scip indexes found in %s", scipDir)
	}

	mergedScip, err := idx.mergeInputs(inputs, mergedPath)
	if err != nil {
		return idx.Status(), err
	}
	idx.publish(idx.Status(), "progress", "", "merged indexes")

	if err := idx.convertToSQLite(mergedScip, indexPath); err != nil {
		return idx.Status(), err
	}
	idx.publish(idx.Status(), "progress", "", "converted to sqlite")

	languagesForMetadata := languagesFromInputs(inputs)
	if len(languagesForMetadata) == 0 {
		languagesForMetadata = languages
	}
	if meta, err := idx.buildMetadata(projectRoot, languagesForMetadata); err != nil {
		idx.logWarn("scip metadata build failed", err)
	} else if err := idx.saveMetadata(indexPath, meta); err != nil {
		idx.logWarn("scip metadata write failed", err)
	}

	status := idx.Status()
	status.Languages = normalizeLanguages(languagesForMetadata)
	status.ProjectRoot = projectRoot
	status.MergedScip = mergedScip
	status.IndexPath = indexPath
	status.StartedAt = startedAt
	status.UpdatedAt = idx.now()
	return status, nil
}

func (idx *AsyncIndexer) finish(status IndexStatus, err error) {
	completedAt := idx.now()
	status.CompletedAt = completedAt
	status.Duration = completedAt.Sub(status.StartedAt)
	status.InProgress = false
	status.UpdatedAt = completedAt
	if err != nil {
		status.Error = err.Error()
	}

	callback := idx.onSuccess
	if err == nil && callback != nil {
		if callbackErr := callback(status); callbackErr != nil {
			status.Error = callbackErr.Error()
			err = callbackErr
		}
	}

	idx.mu.Lock()
	idx.running = false
	idx.status = status
	idx.mu.Unlock()

	if err != nil {
		idx.logWarn("scip indexing failed", err)
		idx.publish(status, "error", "", err.Error())
		return
	}
	idx.publish(status, "complete", "", "scip indexing complete")
}

func (idx *AsyncIndexer) selectLanguages(detected, requested []string) []string {
	detected = normalizeLanguages(detected)
	requested = normalizeLanguages(requested)
	if len(requested) == 0 {
		return detected
	}
	allowed := make(map[string]struct{}, len(detected))
	for _, language := range detected {
		allowed[language] = struct{}{}
	}
	filtered := make([]string, 0, len(requested))
	for _, language := range requested {
		if _, ok := allowed[language]; ok {
			filtered = append(filtered, language)
		}
	}
	return filtered
}

func (idx *AsyncIndexer) mergeInputs(inputs []string, mergedPath string) (string, error) {
	if len(inputs) == 1 && inputs[0] == mergedPath {
		return mergedPath, nil
	}
	tempPath := mergedPath + ".tmp"
	_ = os.Remove(tempPath)

	if len(inputs) == 1 {
		if err := copyFile(inputs[0], tempPath); err != nil {
			return mergedPath, err
		}
	} else {
		if err := idx.mergeIndexes(inputs, tempPath); err != nil {
			return mergedPath, err
		}
	}
	if err := replaceFile(tempPath, mergedPath); err != nil {
		return mergedPath, err
	}
	return mergedPath, nil
}

func (idx *AsyncIndexer) publish(status IndexStatus, eventType, language, message string) {
	_ = status
	bus := idx.eventBus
	if bus == nil {
		return
	}
	event := eventtypes.SCIPEvent{
		EventType:  eventType,
		Language:   strings.TrimSpace(language),
		Message:    strings.TrimSpace(message),
		OccurredAt: idx.now(),
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	bus.Publish(event)
}

func (idx *AsyncIndexer) logWarn(message string, err error) {
	if idx.logger == nil {
		return
	}
	fields := map[string]string{
		"gestalt.category": "scip",
		"gestalt.source":   "backend",
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	idx.logger.Warn(message, fields)
}

func collectScipInputs(scipDir string, allowedLanguages []string) ([]string, error) {
	entries, err := os.ReadDir(scipDir)
	if err != nil {
		return nil, fmt.Errorf("read scip dir: %w", err)
	}
	allowed := make(map[string]struct{})
	for _, language := range normalizeLanguages(allowedLanguages) {
		allowed[language] = struct{}{}
	}
	filterByLanguage := len(allowed) > 0
	inputs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".scip") {
			continue
		}
		if name == mergedScipName || strings.HasSuffix(name, ".tmp") {
			continue
		}
		path := filepath.Join(scipDir, name)
		if filterByLanguage {
			language := languageFromScipFile(path)
			if language == "" {
				continue
			}
			if _, ok := allowed[language]; !ok {
				continue
			}
		}
		inputs = append(inputs, path)
	}
	sort.Strings(inputs)
	return inputs, nil
}

func languagesFromInputs(inputs []string) []string {
	languages := make([]string, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, input := range inputs {
		language := languageFromScipFile(input)
		if language == "" {
			continue
		}
		if _, ok := seen[language]; ok {
			continue
		}
		seen[language] = struct{}{}
		languages = append(languages, language)
	}
	return normalizeLanguages(languages)
}

func languageFromScipFile(path string) string {
	base := filepath.Base(path)
	if base == mergedScipName {
		return "default"
	}
	if !strings.HasPrefix(base, "index-") || !strings.HasSuffix(base, ".scip") {
		return ""
	}
	language := strings.TrimSuffix(strings.TrimPrefix(base, "index-"), ".scip")
	return strings.ToLower(strings.TrimSpace(language))
}

func indexFileName(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" || language == "default" {
		return mergedScipName
	}
	return "index-" + language + ".scip"
}

func copyFile(source, destination string) error {
	payload, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read scip index %s: %w", source, err)
	}
	if err := os.WriteFile(destination, payload, 0o644); err != nil {
		return fmt.Errorf("write scip index %s: %w", destination, err)
	}
	return nil
}

func replaceFile(tempPath, destination string) error {
	if err := os.Rename(tempPath, destination); err == nil {
		return nil
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing file: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}
