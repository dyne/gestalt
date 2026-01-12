package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gestalt"
	"gestalt/internal/agent"
	"gestalt/internal/api"
	"gestalt/internal/config"
	"gestalt/internal/event"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/skill"
	"gestalt/internal/temporal"
	temporalworker "gestalt/internal/temporal/worker"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

type Config struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	AuthToken            string
	SCIPIndexPath        string
	TemporalHost         string
	TemporalNamespace    string
	TemporalEnabled      bool
	TemporalDevServer    bool
	TemporalUIPort       int
	SessionRetentionDays int
	SessionPersist       bool
	SessionLogDir        string
	SessionBufferLines   int
	InputHistoryPersist  bool
	InputHistoryDir      string
	ConfigDir            string
	ConfigBackupLimit    int
	DevMode              bool
	MaxWatches           int
	Verbose              bool
	Quiet                bool
	ShowVersion          bool
	ForceUpgrade         bool
	Sources              map[string]configSource
}

type configSource string

const (
	sourceDefault configSource = "default"
	sourceEnv     configSource = "env"
	sourceFlag    configSource = "flag"
)

const temporalDefaultHost = "localhost:7233"
const temporalHealthCheckTimeout = 500 * time.Millisecond
const temporalDevServerStartTimeout = 10 * time.Second
const temporalDevServerStopTimeout = 5 * time.Second
const httpServerShutdownTimeout = 5 * time.Second

type configDefaults struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	AuthToken            string
	SCIPIndexPath        string
	TemporalHost         string
	TemporalNamespace    string
	TemporalEnabled      bool
	TemporalDevServer    bool
	SessionRetentionDays int
	SessionPersist       bool
	SessionLogDir        string
	SessionBufferLines   int
	InputHistoryPersist  bool
	InputHistoryDir      string
	ConfigDir            string
	ConfigBackupLimit    int
	DevMode              bool
	MaxWatches           int
	ForceUpgrade         bool
}

type flagValues struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	Token                string
	TemporalHost         string
	TemporalNamespace    string
	TemporalEnabled      bool
	TemporalDevServer    bool
	SessionRetentionDays int
	SessionPersist       bool
	SessionLogDir        string
	SessionBufferLines   int
	InputHistoryPersist  bool
	InputHistoryDir      string
	MaxWatches           int
	Verbose              bool
	Quiet                bool
	Help                 bool
	Version              bool
	ForceUpgrade         bool
	DevMode              bool
	Set                  map[string]bool
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "validate-skill" {
		os.Exit(runValidateSkill(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "completion" {
		os.Exit(runCompletion(os.Args[2:], os.Stdout, os.Stderr))
	}
	if hasFlag(os.Args[1:], "--extract-config") {
		os.Exit(runExtractConfig())
	}

	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.ShowVersion {
		if version.Version == "" || version.Version == "dev" {
			fmt.Fprintln(os.Stdout, "gestalt dev")
		} else {
			fmt.Fprintf(os.Stdout, "gestalt version %s\n", version.Version)
		}
		return
	}
	logBuffer := logging.NewLogBuffer(logging.DefaultBufferSize)
	logLevel := logging.LevelInfo
	if cfg.Verbose {
		logLevel = logging.LevelDebug
	} else if cfg.Quiet {
		logLevel = logging.LevelWarning
	}
	logger := logging.NewLogger(logBuffer, logLevel)
	if cfg.Verbose {
		logStartupFlags(logger, cfg)
	}
	logVersionInfo(logger)
	ensureStateDir(cfg, logger)

	temporalDevServer, devServerError := startTemporalDevServer(&cfg, logger)
	if devServerError != nil {
		logger.Warn("temporal dev server start failed", map[string]string{
			"error": devServerError.Error(),
		})
	}
	if temporalDevServer != nil {
		defer temporalDevServer.Stop(logger)
	}
	if cfg.TemporalDevServer && !cfg.TemporalEnabled {
		logger.Warn("temporal dev server running while workflows disabled", nil)
	}

	temporalEnabled := cfg.TemporalEnabled
	var temporalClient temporal.WorkflowClient
	if temporalEnabled {
		if temporalDevServer != nil {
			waitForTemporalServer(cfg.TemporalHost, temporalDevServerStartTimeout, temporalDevServer.Done(), logger)
		} else {
			logTemporalServerHealth(logger, cfg.TemporalHost)
		}
		var temporalClientError error
		temporalClient, temporalClientError = temporal.NewClient(temporal.ClientConfig{
			HostPort:  cfg.TemporalHost,
			Namespace: cfg.TemporalNamespace,
		})
		if temporalClientError != nil {
			temporalEnabled = false
			logger.Warn("temporal client unavailable", map[string]string{
				"host":      cfg.TemporalHost,
				"namespace": cfg.TemporalNamespace,
				"error":     temporalClientError.Error(),
			})
		} else if temporalClient != nil {
			defer temporalClient.Close()
			logger.Info("temporal client connected", map[string]string{
				"host":      cfg.TemporalHost,
				"namespace": cfg.TemporalNamespace,
			})
		}
	}

	configPaths, err := prepareConfig(cfg, logger)
	if err != nil {
		logger.Error("config extraction failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if !cfg.DevMode {
		validatePromptFiles(configPaths.ConfigDir, logger)
	}

	configFS := buildConfigFS(configPaths.Root)
	skills, err := loadSkills(logger, configFS, configPaths.SubDir)
	if err != nil {
		logger.Error("load skills failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("skills loaded", map[string]string{
		"count": strconv.Itoa(len(skills)),
	})

	agents, err := loadAgents(logger, configFS, configPaths.SubDir, buildSkillIndex(skills))
	if err != nil {
		logger.Error("load agents failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("agents loaded", map[string]string{
		"count": strconv.Itoa(len(agents)),
	})

	manager := terminal.NewManager(terminal.ManagerOptions{
		Shell:                cfg.Shell,
		Agents:               agents,
		Skills:               skills,
		Logger:               logger,
		TemporalClient:       temporalClient,
		TemporalEnabled:      temporalEnabled,
		SessionLogDir:        cfg.SessionLogDir,
		InputHistoryDir:      cfg.InputHistoryDir,
		SessionRetentionDays: cfg.SessionRetentionDays,
		BufferLines:          cfg.SessionBufferLines,
		PromptFS:             configFS,
		PromptDir:            path.Join(configPaths.SubDir, "prompts"),
	})

	workerStarted := false
	if temporalEnabled && temporalClient != nil {
		workerError := temporalworker.StartWorker(temporalClient, manager)
		if workerError != nil {
			logger.Warn("temporal worker start failed", map[string]string{
				"error": workerError.Error(),
			})
		} else {
			workerStarted = true
		}
	}
	if workerStarted {
		defer temporalworker.StopWorker()
	}

	planPath := preparePlanFile(logger)

	fsWatcher, err := watcher.NewWithOptions(watcher.Options{
		Logger:     logger,
		MaxWatches: cfg.MaxWatches,
	})
	if err != nil && logger != nil {
		logger.Warn("filesystem watcher unavailable", map[string]string{
			"error": err.Error(),
		})
	}
	eventBus := event.NewBus[watcher.Event](context.Background(), event.BusOptions{
		Name: "watcher_events",
	})
	if fsWatcher != nil {
		fsWatcher.SetErrorHandler(func(err error) {
			eventBus.Publish(watcher.Event{
				Type:      watcher.EventTypeWatchError,
				Timestamp: time.Now().UTC(),
			})
		})
		watchPlanFile(eventBus, fsWatcher, logger, planPath)
		if workDir, err := os.Getwd(); err == nil {
			if _, err := watcher.StartGitWatcher(eventBus, fsWatcher, workDir); err != nil && logger != nil {
				logger.Warn("git watcher unavailable", map[string]string{
					"error": err.Error(),
				})
			}
		} else if logger != nil {
			logger.Warn("git watcher unavailable", map[string]string{
				"error": err.Error(),
			})
		}
	}

	staticDir := findStaticDir()
	frontendFS := fs.FS(nil)
	if sub, err := fs.Sub(gestalt.EmbeddedFrontendFS, path.Join("frontend", "dist")); err == nil {
		frontendFS = sub
	} else if logger != nil {
		logger.Warn("embedded frontend unavailable", map[string]string{
			"error": err.Error(),
		})
	}

	backendMux := http.NewServeMux()
	api.RegisterRoutes(backendMux, manager, cfg.AuthToken, api.StatusConfig{
		TemporalUIPort: cfg.TemporalUIPort,
	}, cfg.SCIPIndexPath, "", nil, logger, eventBus)
	backendListener, backendPort, err := listenOnPort(cfg.BackendPort)
	if err != nil {
		logger.Error("backend listen failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	cfg.BackendPort = backendPort
	backendServer := &http.Server{
		Handler:           backendMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	backendAddress := backendListener.Addr().String()
	logger.Info("gestalt backend listening", map[string]string{
		"addr":    backendAddress,
		"version": version.Version,
	})

	backendURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", backendPort),
	}
	frontendHandler := buildFrontendHandler(staticDir, frontendFS, backendURL, cfg.AuthToken, logger)
	frontendServer := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.FrontendPort),
		Handler:           frontendHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("gestalt frontend listening", map[string]string{
		"addr":    frontendServer.Addr,
		"version": version.Version,
	})

	serverErrors := make(chan serverError, 2)
	go func() {
		serverErrors <- serverError{name: "backend", err: backendServer.Serve(backendListener)}
	}()
	go func() {
		serverErrors <- serverError{name: "frontend", err: frontendServer.ListenAndServe()}
	}()

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)

	var initialError *serverError
	select {
	case err := <-serverErrors:
		initialError = &err
	case sig := <-stopSignals:
		logger.Info("shutdown signal received", map[string]string{
			"signal": sig.String(),
		})
	}

	logServerError(logger, initialError)
	shutdownContext, cancel := context.WithTimeout(context.Background(), httpServerShutdownTimeout)
	defer cancel()
	if err := backendServer.Shutdown(shutdownContext); err != nil {
		logger.Warn("backend server shutdown failed", map[string]string{
			"error": err.Error(),
		})
	}
	if err := frontendServer.Shutdown(shutdownContext); err != nil {
		logger.Warn("frontend server shutdown failed", map[string]string{
			"error": err.Error(),
		})
	}
	drainServerErrors(serverErrors, logger, initialError != nil)
}

type serverError struct {
	name string
	err  error
}

func logServerError(logger *logging.Logger, serverErr *serverError) {
	if logger == nil || serverErr == nil || serverErr.err == nil {
		return
	}
	if errors.Is(serverErr.err, http.ErrServerClosed) {
		return
	}
	logger.Error("http server stopped", map[string]string{
		"server": serverErr.name,
		"error":  serverErr.err.Error(),
	})
}

func drainServerErrors(errorsChan <-chan serverError, logger *logging.Logger, initialLogged bool) {
	pending := 2
	if initialLogged {
		pending = 1
	}
	for i := 0; i < pending; i++ {
		select {
		case err := <-errorsChan:
			logServerError(logger, &err)
		case <-time.After(httpServerShutdownTimeout):
			return
		}
	}
}

func listenOnPort(port int) (net.Listener, int, error) {
	address := ":" + strconv.Itoa(port)
	if port == 0 {
		address = ":0"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, 0, err
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, fmt.Errorf("unexpected listener address: %T", listener.Addr())
	}
	return listener, tcpAddress.Port, nil
}

func buildFrontendHandler(staticDir string, frontendFS fs.FS, backendURL *url.URL, authToken string, logger *logging.Logger) http.Handler {
	mux := http.NewServeMux()
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if logger != nil {
			logger.Warn("frontend proxy error", map[string]string{
				"error": err.Error(),
			})
		}
		http.Error(w, "backend unavailable", http.StatusBadGateway)
	}

	mux.Handle("/api", proxy)
	mux.Handle("/api/", proxy)
	mux.Handle("/ws", proxy)
	mux.Handle("/ws/", proxy)

	if staticDir != "" {
		mux.Handle("/", api.NewSPAHandler(staticDir))
		return mux
	}

	if frontendFS != nil {
		mux.Handle("/", api.NewSPAHandlerFS(frontendFS))
		return mux
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authToken != "" {
			w.Header().Set("X-Gestalt-Auth", "required")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gestalt ok\n"))
	})
	return mux
}

func watchPlanFile(bus *event.Bus[watcher.Event], watch watcher.Watch, logger *logging.Logger, planPath string) {
	if bus == nil || watch == nil {
		return
	}
	if planPath == "" {
		planPath = plan.DefaultPath()
	}

	var retryMutex sync.Mutex
	retrying := false
	var handleMu sync.Mutex
	var handle watcher.Handle

	stopWatch := func() {
		handleMu.Lock()
		if handle != nil {
			_ = handle.Close()
			handle = nil
		}
		handleMu.Unlock()
	}

	startWatch := func() error {
		newHandle, err := watcher.WatchFile(bus, watch, planPath)
		if err != nil {
			return err
		}
		handleMu.Lock()
		if handle != nil {
			_ = handle.Close()
		}
		handle = newHandle
		handleMu.Unlock()
		return nil
	}

	startRetry := func() {
		retryMutex.Lock()
		if retrying {
			retryMutex.Unlock()
			return
		}
		retrying = true
		retryMutex.Unlock()

		go func() {
			defer func() {
				retryMutex.Lock()
				retrying = false
				retryMutex.Unlock()
			}()
			backoff := 100 * time.Millisecond
			for {
				if err := startWatch(); err == nil {
					if logger != nil {
						logger.Info("Watching plan file for changes", map[string]string{
							"path": planPath,
						})
					}
					return
				}
				time.Sleep(backoff)
				if backoff < 2*time.Second {
					backoff *= 2
				}
			}
		}()
	}

	if err := startWatch(); err != nil {
		if logger != nil {
			logger.Warn("plan watch failed", map[string]string{
				"path":  planPath,
				"error": err.Error(),
			})
		}
		startRetry()
	} else if logger != nil {
		logger.Info("Watching plan file for changes", map[string]string{
			"path": planPath,
		})
	}

	events, _ := bus.SubscribeFiltered(func(event watcher.Event) bool {
		return event.Type == watcher.EventTypeFileChanged && event.Path == planPath
	})
	go func() {
		for event := range events {
			if event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			stopWatch()
			startRetry()
		}
	}()
}

func loadConfig(args []string) (Config, error) {
	defaults := defaultConfigValues()
	flags, err := parseFlags(args, defaults)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Sources: make(map[string]configSource),
	}

	frontendPort := defaults.FrontendPort
	frontendPortSource := sourceDefault
	if rawPort := os.Getenv("GESTALT_PORT"); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
			frontendPort = parsed
			frontendPortSource = sourceEnv
		}
	}
	if flags.Set["port"] {
		if flags.FrontendPort <= 0 {
			return Config{}, fmt.Errorf("invalid --port: must be > 0")
		}
		frontendPort = flags.FrontendPort
		frontendPortSource = sourceFlag
	}
	cfg.FrontendPort = frontendPort
	cfg.Sources["port"] = frontendPortSource

	backendPort := defaults.BackendPort
	backendPortSource := sourceDefault
	if rawPort := os.Getenv("GESTALT_BACKEND_PORT"); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
			backendPort = parsed
			backendPortSource = sourceEnv
		}
	}
	if flags.Set["backend-port"] {
		if flags.BackendPort <= 0 {
			return Config{}, fmt.Errorf("invalid --backend-port: must be > 0")
		}
		backendPort = flags.BackendPort
		backendPortSource = sourceFlag
	}
	cfg.BackendPort = backendPort
	cfg.Sources["backend-port"] = backendPortSource

	shell := defaults.Shell
	shellSource := sourceDefault
	if rawShell := strings.TrimSpace(os.Getenv("GESTALT_SHELL")); rawShell != "" {
		shell = rawShell
		shellSource = sourceEnv
	}
	if flags.Set["shell"] {
		trimmed := strings.TrimSpace(flags.Shell)
		if trimmed == "" {
			return Config{}, fmt.Errorf("invalid --shell: value cannot be empty")
		}
		shell = trimmed
		shellSource = sourceFlag
	}
	cfg.Shell = shell
	cfg.Sources["shell"] = shellSource

	token := os.Getenv("GESTALT_TOKEN")
	tokenSource := sourceDefault
	if token != "" {
		tokenSource = sourceEnv
	}
	if flags.Set["token"] {
		token = flags.Token
		tokenSource = sourceFlag
	}
	cfg.AuthToken = token
	cfg.Sources["token"] = tokenSource

	scipIndexPath := defaults.SCIPIndexPath
	scipIndexPathSource := sourceDefault
	if rawIndexPath := strings.TrimSpace(os.Getenv("GESTALT_SCIP_INDEX_PATH")); rawIndexPath != "" {
		scipIndexPath = rawIndexPath
		scipIndexPathSource = sourceEnv
	}
	cfg.SCIPIndexPath = scipIndexPath
	cfg.Sources["scip-index-path"] = scipIndexPathSource

	temporalHost := defaults.TemporalHost
	temporalHostSource := sourceDefault
	if rawHost := strings.TrimSpace(os.Getenv("GESTALT_TEMPORAL_HOST")); rawHost != "" {
		temporalHost = rawHost
		temporalHostSource = sourceEnv
	}
	if flags.Set["temporal-host"] {
		trimmed := strings.TrimSpace(flags.TemporalHost)
		if trimmed == "" {
			return Config{}, fmt.Errorf("invalid --temporal-host: value cannot be empty")
		}
		temporalHost = trimmed
		temporalHostSource = sourceFlag
	}
	cfg.TemporalHost = temporalHost
	cfg.Sources["temporal-host"] = temporalHostSource

	temporalNamespace := defaults.TemporalNamespace
	temporalNamespaceSource := sourceDefault
	if rawNamespace := strings.TrimSpace(os.Getenv("GESTALT_TEMPORAL_NAMESPACE")); rawNamespace != "" {
		temporalNamespace = rawNamespace
		temporalNamespaceSource = sourceEnv
	}
	if flags.Set["temporal-namespace"] {
		trimmed := strings.TrimSpace(flags.TemporalNamespace)
		if trimmed == "" {
			return Config{}, fmt.Errorf("invalid --temporal-namespace: value cannot be empty")
		}
		temporalNamespace = trimmed
		temporalNamespaceSource = sourceFlag
	}
	cfg.TemporalNamespace = temporalNamespace
	cfg.Sources["temporal-namespace"] = temporalNamespaceSource

	temporalEnabled := defaults.TemporalEnabled
	temporalEnabledSource := sourceDefault
	if rawEnabled := strings.TrimSpace(os.Getenv("GESTALT_TEMPORAL_ENABLED")); rawEnabled != "" {
		if parsed, err := strconv.ParseBool(rawEnabled); err == nil {
			temporalEnabled = parsed
			temporalEnabledSource = sourceEnv
		}
	}
	if flags.Set["temporal-enabled"] {
		temporalEnabled = flags.TemporalEnabled
		temporalEnabledSource = sourceFlag
	}
	cfg.TemporalEnabled = temporalEnabled
	cfg.Sources["temporal-enabled"] = temporalEnabledSource

	temporalDevServer := defaults.TemporalDevServer
	temporalDevServerSource := sourceDefault
	if rawDevServer := strings.TrimSpace(os.Getenv("GESTALT_TEMPORAL_DEV_SERVER")); rawDevServer != "" {
		if parsed, err := strconv.ParseBool(rawDevServer); err == nil {
			temporalDevServer = parsed
			temporalDevServerSource = sourceEnv
		}
	}
	if flags.Set["temporal-dev-server"] {
		temporalDevServer = flags.TemporalDevServer
		temporalDevServerSource = sourceFlag
	}
	cfg.TemporalDevServer = temporalDevServer
	cfg.Sources["temporal-dev-server"] = temporalDevServerSource

	retentionDays := defaults.SessionRetentionDays
	retentionSource := sourceDefault
	if rawRetention := os.Getenv("GESTALT_SESSION_RETENTION_DAYS"); rawRetention != "" {
		if parsed, err := strconv.Atoi(rawRetention); err == nil && parsed > 0 {
			retentionDays = parsed
			retentionSource = sourceEnv
		}
	}
	if flags.Set["session-retention-days"] {
		if flags.SessionRetentionDays <= 0 {
			return Config{}, fmt.Errorf("invalid --session-retention-days: must be > 0")
		}
		retentionDays = flags.SessionRetentionDays
		retentionSource = sourceFlag
	}
	cfg.SessionRetentionDays = retentionDays
	cfg.Sources["session-retention-days"] = retentionSource

	sessionPersist := defaults.SessionPersist
	sessionPersistSource := sourceDefault
	if rawPersist := os.Getenv("GESTALT_SESSION_PERSIST"); rawPersist != "" {
		if parsed, err := strconv.ParseBool(rawPersist); err == nil {
			sessionPersist = parsed
			sessionPersistSource = sourceEnv
		}
	}
	if flags.Set["session-persist"] {
		sessionPersist = flags.SessionPersist
		sessionPersistSource = sourceFlag
	}
	cfg.SessionPersist = sessionPersist
	cfg.Sources["session-persist"] = sessionPersistSource

	sessionLogDir := ""
	sessionDirSource := sessionPersistSource
	if sessionPersist {
		sessionLogDir = defaults.SessionLogDir
		sessionDirSource = sourceDefault
		if rawDir := strings.TrimSpace(os.Getenv("GESTALT_SESSION_DIR")); rawDir != "" {
			sessionLogDir = rawDir
			sessionDirSource = sourceEnv
		}
		if flags.Set["session-dir"] {
			trimmed := strings.TrimSpace(flags.SessionLogDir)
			if trimmed == "" {
				return Config{}, fmt.Errorf("invalid --session-dir: value cannot be empty")
			}
			sessionLogDir = trimmed
			sessionDirSource = sourceFlag
		}
	}
	cfg.SessionLogDir = sessionLogDir
	cfg.Sources["session-dir"] = sessionDirSource

	sessionBufferLines := defaults.SessionBufferLines
	sessionBufferSource := sourceDefault
	if rawLines := os.Getenv("GESTALT_SESSION_BUFFER_LINES"); rawLines != "" {
		if parsed, err := strconv.Atoi(rawLines); err == nil && parsed > 0 {
			sessionBufferLines = parsed
			sessionBufferSource = sourceEnv
		}
	}
	if flags.Set["session-buffer-lines"] {
		if flags.SessionBufferLines <= 0 {
			return Config{}, fmt.Errorf("invalid --session-buffer-lines: must be > 0")
		}
		sessionBufferLines = flags.SessionBufferLines
		sessionBufferSource = sourceFlag
	}
	cfg.SessionBufferLines = sessionBufferLines
	cfg.Sources["session-buffer-lines"] = sessionBufferSource

	inputHistoryPersist := defaults.InputHistoryPersist
	inputHistoryPersistSource := sourceDefault
	if rawPersist := os.Getenv("GESTALT_INPUT_HISTORY_PERSIST"); rawPersist != "" {
		if parsed, err := strconv.ParseBool(rawPersist); err == nil {
			inputHistoryPersist = parsed
			inputHistoryPersistSource = sourceEnv
		}
	}
	if flags.Set["input-history-persist"] {
		inputHistoryPersist = flags.InputHistoryPersist
		inputHistoryPersistSource = sourceFlag
	}
	cfg.InputHistoryPersist = inputHistoryPersist
	cfg.Sources["input-history-persist"] = inputHistoryPersistSource

	inputHistoryDir := ""
	inputHistoryDirSource := inputHistoryPersistSource
	if inputHistoryPersist {
		inputHistoryDir = defaults.InputHistoryDir
		inputHistoryDirSource = sourceDefault
		if rawDir := strings.TrimSpace(os.Getenv("GESTALT_INPUT_HISTORY_DIR")); rawDir != "" {
			inputHistoryDir = rawDir
			inputHistoryDirSource = sourceEnv
		}
		if flags.Set["input-history-dir"] {
			trimmed := strings.TrimSpace(flags.InputHistoryDir)
			if trimmed == "" {
				return Config{}, fmt.Errorf("invalid --input-history-dir: value cannot be empty")
			}
			inputHistoryDir = trimmed
			inputHistoryDirSource = sourceFlag
		}
	}
	cfg.InputHistoryDir = inputHistoryDir
	cfg.Sources["input-history-dir"] = inputHistoryDirSource

	devMode := defaults.DevMode
	devModeSource := sourceDefault
	if rawDev := strings.TrimSpace(os.Getenv("GESTALT_DEV_MODE")); rawDev != "" {
		if parsed, err := strconv.ParseBool(rawDev); err == nil {
			devMode = parsed
			devModeSource = sourceEnv
		}
	}
	if flags.Set["dev"] {
		devMode = flags.DevMode
		devModeSource = sourceFlag
	}
	cfg.DevMode = devMode
	cfg.Sources["dev-mode"] = devModeSource

	configDir := defaults.ConfigDir
	configDirSource := sourceDefault
	if rawDir := strings.TrimSpace(os.Getenv("GESTALT_CONFIG_DIR")); rawDir != "" {
		configDir = rawDir
		configDirSource = sourceEnv
	}
	if devMode && configDirSource == sourceDefault {
		configDir = "config"
	}
	if strings.TrimSpace(configDir) == "" {
		return Config{}, fmt.Errorf("invalid config dir: value cannot be empty")
	}
	cfg.ConfigDir = configDir
	cfg.Sources["config-dir"] = configDirSource

	configBackupLimit := defaults.ConfigBackupLimit
	configBackupLimitSource := sourceDefault
	if rawLimit := strings.TrimSpace(os.Getenv("GESTALT_CONFIG_BACKUP_LIMIT")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed >= 0 {
			configBackupLimit = parsed
			configBackupLimitSource = sourceEnv
		}
	}
	cfg.ConfigBackupLimit = configBackupLimit
	cfg.Sources["config-backup-limit"] = configBackupLimitSource

	maxWatches := defaults.MaxWatches
	maxWatchesSource := sourceDefault
	if rawMax := strings.TrimSpace(os.Getenv("GESTALT_MAX_WATCHES")); rawMax != "" {
		if parsed, err := strconv.Atoi(rawMax); err == nil && parsed > 0 {
			maxWatches = parsed
			maxWatchesSource = sourceEnv
		}
	}
	if flags.Set["max-watches"] {
		if flags.MaxWatches <= 0 {
			return Config{}, fmt.Errorf("invalid --max-watches: must be > 0")
		}
		maxWatches = flags.MaxWatches
		maxWatchesSource = sourceFlag
	}
	cfg.MaxWatches = maxWatches
	cfg.Sources["max-watches"] = maxWatchesSource

	verboseSource := sourceDefault
	if flags.Set["verbose"] {
		cfg.Verbose = flags.Verbose
		verboseSource = sourceFlag
	}
	cfg.Sources["verbose"] = verboseSource

	quietSource := sourceDefault
	if flags.Set["quiet"] {
		cfg.Quiet = flags.Quiet
		quietSource = sourceFlag
	}
	cfg.Sources["quiet"] = quietSource

	versionSource := sourceDefault
	cfg.ShowVersion = flags.Version
	if flags.Set["version"] {
		versionSource = sourceFlag
	}
	cfg.Sources["version"] = versionSource

	forceUpgradeSource := sourceDefault
	cfg.ForceUpgrade = defaults.ForceUpgrade
	if flags.Set["force-upgrade"] {
		cfg.ForceUpgrade = flags.ForceUpgrade
		forceUpgradeSource = sourceFlag
	}
	cfg.Sources["force-upgrade"] = forceUpgradeSource

	return cfg, nil
}

func defaultConfigValues() configDefaults {
	return configDefaults{
		FrontendPort:         57417,
		BackendPort:          0,
		Shell:                terminal.DefaultShell(),
		AuthToken:            "",
		SCIPIndexPath:        filepath.Join(".gestalt", "index.db"),
		TemporalHost:         temporalDefaultHost,
		TemporalNamespace:    "default",
		TemporalEnabled:      true,
		TemporalDevServer:    true,
		SessionRetentionDays: terminal.DefaultSessionRetentionDays,
		SessionPersist:       true,
		SessionLogDir:        filepath.Join(".gestalt", "sessions"),
		SessionBufferLines:   terminal.DefaultBufferLines,
		InputHistoryPersist:  true,
		InputHistoryDir:      filepath.Join(".gestalt", "input-history"),
		ConfigDir:            filepath.Join(".gestalt", "config"),
		ConfigBackupLimit:    1,
		DevMode:              false,
		MaxWatches:           100,
		ForceUpgrade:         false,
	}
}

func parseFlags(args []string, defaults configDefaults) (flagValues, error) {
	if args == nil {
		args = []string{}
	}
	fs := flag.NewFlagSet("gestalt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	port := fs.Int("port", defaults.FrontendPort, "HTTP frontend port")
	backendPort := fs.Int("backend-port", defaults.BackendPort, "Backend API port")
	shell := fs.String("shell", defaults.Shell, "Default shell command")
	token := fs.String("token", defaults.AuthToken, "Auth token for REST/WS")
	temporalHost := fs.String("temporal-host", defaults.TemporalHost, "Temporal server host:port")
	temporalNamespace := fs.String("temporal-namespace", defaults.TemporalNamespace, "Temporal namespace")
	temporalEnabled := fs.Bool("temporal-enabled", defaults.TemporalEnabled, "Enable Temporal workflows")
	temporalDevServer := fs.Bool("temporal-dev-server", defaults.TemporalDevServer, "Auto-start Temporal dev server")
	sessionPersist := fs.Bool("session-persist", defaults.SessionPersist, "Persist terminal sessions to disk")
	sessionDir := fs.String("session-dir", defaults.SessionLogDir, "Session log directory")
	sessionRetentionDays := fs.Int("session-retention-days", defaults.SessionRetentionDays, "Session retention days")
	sessionBufferLines := fs.Int("session-buffer-lines", defaults.SessionBufferLines, "Session buffer lines")
	inputHistoryPersist := fs.Bool("input-history-persist", defaults.InputHistoryPersist, "Persist input history")
	inputHistoryDir := fs.String("input-history-dir", defaults.InputHistoryDir, "Input history directory")
	maxWatches := fs.Int("max-watches", defaults.MaxWatches, "Max active watches")
	forceUpgrade := fs.Bool("force-upgrade", defaults.ForceUpgrade, "Bypass config version compatibility checks")
	devMode := fs.Bool("dev", defaults.DevMode, "Enable developer mode (skip config extraction)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	quiet := fs.Bool("quiet", false, "Reduce logging to warnings")
	help := fs.Bool("help", false, "Show help")
	version := fs.Bool("version", false, "Print version and exit")
	helpShort := fs.Bool("h", false, "Show help")
	versionShort := fs.Bool("v", false, "Print version and exit")

	fs.Usage = func() {
		printHelp(fs.Output(), defaults)
	}

	if err := fs.Parse(args); err != nil {
		return flagValues{}, err
	}

	set := make(map[string]bool)
	fs.Visit(func(flag *flag.Flag) {
		set[flag.Name] = true
	})

	flags := flagValues{
		FrontendPort:         *port,
		BackendPort:          *backendPort,
		Shell:                *shell,
		Token:                *token,
		TemporalHost:         *temporalHost,
		TemporalNamespace:    *temporalNamespace,
		TemporalEnabled:      *temporalEnabled,
		TemporalDevServer:    *temporalDevServer,
		SessionRetentionDays: *sessionRetentionDays,
		SessionPersist:       *sessionPersist,
		SessionLogDir:        *sessionDir,
		SessionBufferLines:   *sessionBufferLines,
		InputHistoryPersist:  *inputHistoryPersist,
		InputHistoryDir:      *inputHistoryDir,
		MaxWatches:           *maxWatches,
		ForceUpgrade:         *forceUpgrade,
		DevMode:              *devMode,
		Verbose:              *verbose,
		Quiet:                *quiet,
		Help:                 *help || *helpShort,
		Version:              *version || *versionShort,
		Set:                  set,
	}

	if flags.Help {
		set["help"] = true
		fs.SetOutput(os.Stdout)
		fs.Usage()
		return flags, flag.ErrHelp
	}

	if flags.Version {
		set["version"] = true
	}

	return flags, nil
}

type helpOption struct {
	Name string
	Desc string
}

func printHelp(out io.Writer, defaults configDefaults) {
	fmt.Fprintln(out, "Usage: gestalt [options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Gestalt multi-terminal dashboard with agent profiles")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")

	writeOptionGroup(out, "Server", []helpOption{
		{
			Name: "--port PORT",
			Desc: fmt.Sprintf("HTTP frontend port (env: GESTALT_PORT, default: %d)", defaults.FrontendPort),
		},
		{
			Name: "--backend-port PORT",
			Desc: "Backend API port (env: GESTALT_BACKEND_PORT, default: random)",
		},
		{
			Name: "--shell SHELL",
			Desc: "Default shell command (env: GESTALT_SHELL, default: system shell)",
		},
		{
			Name: "--token TOKEN",
			Desc: "Auth token for REST/WS (env: GESTALT_TOKEN, default: none)",
		},
	})

	writeOptionGroup(out, "Temporal", []helpOption{
		{
			Name: "--temporal-host HOST:PORT",
			Desc: fmt.Sprintf("Temporal server address (env: GESTALT_TEMPORAL_HOST, default: %s)", defaults.TemporalHost),
		},
		{
			Name: "--temporal-namespace NAME",
			Desc: fmt.Sprintf("Temporal namespace (env: GESTALT_TEMPORAL_NAMESPACE, default: %s)", defaults.TemporalNamespace),
		},
		{
			Name: "--temporal-enabled",
			Desc: fmt.Sprintf("Enable Temporal workflows (env: GESTALT_TEMPORAL_ENABLED, default: %t)", defaults.TemporalEnabled),
		},
		{
			Name: "--temporal-dev-server",
			Desc: fmt.Sprintf("Auto-start Temporal dev server (env: GESTALT_TEMPORAL_DEV_SERVER, default: %t)", defaults.TemporalDevServer),
		},
	})

	writeOptionGroup(out, "Sessions", []helpOption{
		{
			Name: "--session-persist",
			Desc: fmt.Sprintf("Persist terminal sessions to disk (env: GESTALT_SESSION_PERSIST, default: %t)", defaults.SessionPersist),
		},
		{
			Name: "--session-dir DIR",
			Desc: fmt.Sprintf("Session log directory (env: GESTALT_SESSION_DIR, default: %s)", defaults.SessionLogDir),
		},
		{
			Name: "--session-buffer-lines N",
			Desc: fmt.Sprintf("Session buffer lines (env: GESTALT_SESSION_BUFFER_LINES, default: %d)", defaults.SessionBufferLines),
		},
		{
			Name: "--session-retention-days DAYS",
			Desc: fmt.Sprintf("Session retention days (env: GESTALT_SESSION_RETENTION_DAYS, default: %d)", defaults.SessionRetentionDays),
		},
	})

	writeOptionGroup(out, "Input history", []helpOption{
		{
			Name: "--input-history-persist",
			Desc: fmt.Sprintf("Persist input history (env: GESTALT_INPUT_HISTORY_PERSIST, default: %t)", defaults.InputHistoryPersist),
		},
		{
			Name: "--input-history-dir DIR",
			Desc: fmt.Sprintf("Input history directory (env: GESTALT_INPUT_HISTORY_DIR, default: %s)", defaults.InputHistoryDir),
		},
	})

	writeOptionGroup(out, "Watching", []helpOption{
		{
			Name: "--max-watches N",
			Desc: fmt.Sprintf("Max active watches (env: GESTALT_MAX_WATCHES, default: %d)", defaults.MaxWatches),
		},
	})

	writeOptionGroup(out, "Common", []helpOption{
		{
			Name: "--verbose",
			Desc: "Enable verbose logging (default: false)",
		},
		{
			Name: "--quiet",
			Desc: "Reduce logging to warnings (default: false)",
		},
		{
			Name: "--force-upgrade",
			Desc: "Bypass config version compatibility checks (dangerous)",
		},
		{
			Name: "--dev",
			Desc: fmt.Sprintf("Enable developer mode (env: GESTALT_DEV_MODE, default: %t)", defaults.DevMode),
		},
		{
			Name: "--extract-config",
			Desc: "No-op (config extraction runs automatically at startup)",
		},
		{
			Name: "--help",
			Desc: "Show this help message",
		},
		{
			Name: "--version",
			Desc: "Print version and exit",
		},
	})

	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  gestalt validate-skill PATH  Validate an Agent Skill directory or SKILL.md file")
	fmt.Fprintln(out, "  gestalt completion SHELL     Generate shell completion script (bash, zsh)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Shell completion:")
	fmt.Fprintln(out, "  gestalt completion bash > /usr/local/share/bash-completion/completions/gestalt")
	fmt.Fprintln(out, "  gestalt completion zsh > /usr/local/share/zsh/site-functions/_gestalt")
	fmt.Fprintln(out, "  source <(gestalt completion bash)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Environment variables override defaults; CLI flags override environment variables.")
}

func writeOptionGroup(out io.Writer, title string, options []helpOption) {
	fmt.Fprintf(out, "  %s:\n", title)
	for _, option := range options {
		fmt.Fprintf(out, "    %-30s %s\n", option.Name, option.Desc)
	}
	fmt.Fprintln(out, "")
}

func logStartupFlags(logger *logging.Logger, cfg Config) {
	if logger == nil || cfg.Sources == nil {
		return
	}
	var flags []string
	if cfg.Sources["port"] == sourceFlag {
		flags = append(flags, fmt.Sprintf("--port %d", cfg.FrontendPort))
	}
	if cfg.Sources["backend-port"] == sourceFlag {
		flags = append(flags, fmt.Sprintf("--backend-port %d", cfg.BackendPort))
	}
	if cfg.Sources["shell"] == sourceFlag {
		flags = append(flags, formatStringFlag("--shell", cfg.Shell))
	}
	if cfg.Sources["token"] == sourceFlag {
		flags = append(flags, formatTokenFlag(cfg.AuthToken))
	}
	if cfg.Sources["temporal-host"] == sourceFlag {
		flags = append(flags, formatStringFlag("--temporal-host", cfg.TemporalHost))
	}
	if cfg.Sources["temporal-namespace"] == sourceFlag {
		flags = append(flags, formatStringFlag("--temporal-namespace", cfg.TemporalNamespace))
	}
	if cfg.Sources["temporal-enabled"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--temporal-enabled", cfg.TemporalEnabled))
	}
	if cfg.Sources["temporal-dev-server"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--temporal-dev-server", cfg.TemporalDevServer))
	}
	if cfg.Sources["session-persist"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--session-persist", cfg.SessionPersist))
	}
	if cfg.Sources["session-dir"] == sourceFlag {
		flags = append(flags, formatStringFlag("--session-dir", cfg.SessionLogDir))
	}
	if cfg.Sources["session-buffer-lines"] == sourceFlag {
		flags = append(flags, fmt.Sprintf("--session-buffer-lines %d", cfg.SessionBufferLines))
	}
	if cfg.Sources["session-retention-days"] == sourceFlag {
		flags = append(flags, fmt.Sprintf("--session-retention-days %d", cfg.SessionRetentionDays))
	}
	if cfg.Sources["input-history-persist"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--input-history-persist", cfg.InputHistoryPersist))
	}
	if cfg.Sources["input-history-dir"] == sourceFlag {
		flags = append(flags, formatStringFlag("--input-history-dir", cfg.InputHistoryDir))
	}
	if cfg.Sources["max-watches"] == sourceFlag {
		flags = append(flags, fmt.Sprintf("--max-watches %d", cfg.MaxWatches))
	}
	if cfg.Sources["verbose"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--verbose", cfg.Verbose))
	}
	if cfg.Sources["quiet"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--quiet", cfg.Quiet))
	}
	if cfg.Sources["force-upgrade"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--force-upgrade", cfg.ForceUpgrade))
	}
	if cfg.Sources["dev-mode"] == sourceFlag {
		flags = append(flags, "--dev")
	}

	if len(flags) == 0 {
		return
	}
	logger.Debug("starting with flags", map[string]string{
		"flags": strings.Join(flags, " "),
	})
}

func logVersionInfo(logger *logging.Logger) {
	if logger == nil {
		return
	}
	info := version.GetVersionInfo()
	label := formatVersionInfo(info)
	message := fmt.Sprintf("Gestalt version %s", label)
	var details []string
	if info.Built != "" {
		details = append(details, fmt.Sprintf("built %s", info.Built))
	}
	if info.GitCommit != "" {
		details = append(details, fmt.Sprintf("commit %s", info.GitCommit))
	}
	if len(details) > 0 {
		message = fmt.Sprintf("%s (%s)", message, strings.Join(details, ", "))
	}
	logger.Info(message, nil)
}

func formatBoolFlag(name string, value bool) string {
	if value {
		return name
	}
	return fmt.Sprintf("%s=%t", name, value)
}

func formatStringFlag(name, value string) string {
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf("%s=\"\"", name)
	}
	return fmt.Sprintf("%s %s", name, value)
}

func formatTokenFlag(token string) string {
	if strings.TrimSpace(token) == "" {
		return "--token=\"\""
	}
	return "--token [set]"
}

func ensureStateDir(cfg Config, logger *logging.Logger) {
	stateRoot := ".gestalt"
	if !usesStateRoot(cfg.SessionLogDir, stateRoot) &&
		!usesStateRoot(cfg.InputHistoryDir, stateRoot) &&
		!usesStateRoot(cfg.ConfigDir, stateRoot) &&
		!usesStateRoot(cfg.SCIPIndexPath, stateRoot) {
		return
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil && logger != nil {
		logger.Warn("create state dir failed", map[string]string{
			"path":  stateRoot,
			"error": err.Error(),
		})
	}
}

type temporalDevServer struct {
	cmd     *exec.Cmd
	logFile *os.File
	done    chan error
}

func startTemporalDevServer(cfg *Config, logger *logging.Logger) (*temporalDevServer, error) {
	if cfg == nil || !cfg.TemporalDevServer {
		return nil, nil
	}
	temporalPath, err := exec.LookPath("temporal")
	if err != nil {
		return nil, fmt.Errorf("temporal CLI not found")
	}

	dataDir := filepath.Join(".gestalt", "temporal")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		absDataDir = dataDir
	}
	cacheDir := filepath.Join(absDataDir, "cache")
	configDir := filepath.Join(absDataDir, "config")
	stateDir := filepath.Join(absDataDir, "state")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal cache dir: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal config dir: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal state dir: %w", err)
	}

	logPath := filepath.Join(absDataDir, "temporal.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open temporal log: %w", err)
	}

	temporalPort, uiPort, err := resolveTemporalDevPorts(cfg, logger)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}

	cmd := exec.Command(temporalPath, "server", "start-dev",
		"--ip", "0.0.0.0",
		"--port", strconv.Itoa(temporalPort),
		"--ui-port", strconv.Itoa(uiPort),
	)
	cfg.TemporalUIPort = uiPort
	cmd.Dir = absDataDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"HOME="+absDataDir,
		"XDG_CACHE_HOME="+cacheDir,
		"XDG_CONFIG_HOME="+configDir,
		"XDG_STATE_HOME="+stateDir,
	)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start temporal dev server: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if logger != nil {
		logger.Info("temporal dev server started", map[string]string{
			"dir":     absDataDir,
			"log":     logPath,
			"host":    normalizeTemporalHost(cfg.TemporalHost),
			"ui_port": strconv.Itoa(uiPort),
		})
	}

	return &temporalDevServer{
		cmd:     cmd,
		logFile: logFile,
		done:    done,
	}, nil
}

func resolveTemporalDevPorts(cfg *Config, logger *logging.Logger) (int, int, error) {
	if cfg == nil {
		return 0, 0, fmt.Errorf("missing temporal config")
	}
	temporalPort := 0
	temporalHostSource := sourceDefault
	if cfg.Sources != nil {
		temporalHostSource = cfg.Sources["temporal-host"]
	}
	if temporalHostSource != sourceDefault && strings.TrimSpace(cfg.TemporalHost) != "" {
		if _, port, err := net.SplitHostPort(cfg.TemporalHost); err == nil {
			if parsed, err := strconv.Atoi(port); err == nil && parsed > 0 {
				temporalPort = parsed
			}
		} else if logger != nil {
			logger.Warn("temporal host missing port; using random port", map[string]string{
				"host": cfg.TemporalHost,
			})
		}
	}

	if temporalPort == 0 {
		port, err := pickRandomPort()
		if err != nil {
			return 0, 0, fmt.Errorf("select temporal port: %w", err)
		}
		temporalPort = port
		cfg.TemporalHost = fmt.Sprintf("localhost:%d", temporalPort)
	}

	uiPort, err := pickRandomPortExcluding(temporalPort)
	if err != nil {
		return 0, 0, fmt.Errorf("select temporal UI port: %w", err)
	}
	return temporalPort, uiPort, nil
}

func pickRandomPort() (int, error) {
	listener, port, err := listenOnPort(0)
	if err != nil {
		return 0, err
	}
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func pickRandomPortExcluding(excluded int) (int, error) {
	for attempt := 0; attempt < 10; attempt++ {
		port, err := pickRandomPort()
		if err != nil {
			return 0, err
		}
		if port != excluded {
			return port, nil
		}
	}
	return 0, fmt.Errorf("failed to select distinct port")
}

func (server *temporalDevServer) Done() <-chan error {
	if server == nil {
		return nil
	}
	return server.done
}

func (server *temporalDevServer) Stop(logger *logging.Logger) {
	if server == nil {
		return
	}
	if server.cmd == nil || server.cmd.Process == nil {
		if server.logFile != nil {
			_ = server.logFile.Close()
		}
		return
	}

	select {
	case err := <-server.done:
		if logger != nil && err != nil {
			logger.Warn("temporal dev server exited", map[string]string{
				"error": err.Error(),
			})
		}
	default:
		if err := server.cmd.Process.Signal(os.Interrupt); err != nil && logger != nil {
			logger.Warn("temporal dev server signal failed", map[string]string{
				"error": err.Error(),
			})
		}
		select {
		case err := <-server.done:
			if logger != nil && err != nil {
				logger.Warn("temporal dev server stopped", map[string]string{
					"error": err.Error(),
				})
			}
		case <-time.After(temporalDevServerStopTimeout):
			if killErr := server.cmd.Process.Kill(); killErr != nil && logger != nil {
				logger.Warn("temporal dev server kill failed", map[string]string{
					"error": killErr.Error(),
				})
			}
		}
	}

	if server.logFile != nil {
		_ = server.logFile.Close()
	}
}

func logTemporalServerHealth(logger *logging.Logger, host string) {
	if logger == nil {
		return
	}
	if err := temporalServerReachable(host); err != nil {
		logger.Warn("temporal server unavailable", map[string]string{
			"host":  normalizeTemporalHost(host),
			"error": err.Error(),
		})
		return
	}
	logger.Info("temporal server reachable", map[string]string{
		"host": normalizeTemporalHost(host),
	})
}

func waitForTemporalServer(host string, timeout time.Duration, done <-chan error, logger *logging.Logger) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := temporalServerReachable(host); err == nil {
			if logger != nil {
				logger.Info("temporal server ready", map[string]string{
					"host": normalizeTemporalHost(host),
				})
			}
			return true
		}

		if time.Now().After(deadline) {
			if logger != nil {
				logger.Warn("temporal server wait timed out", map[string]string{
					"host": normalizeTemporalHost(host),
				})
			}
			return false
		}

		select {
		case err := <-done:
			if logger != nil {
				message := "temporal dev server exited"
				fields := map[string]string{}
				if err != nil {
					fields["error"] = err.Error()
				}
				logger.Warn(message, fields)
			}
			return false
		case <-ticker.C:
		}
	}
}

func temporalServerReachable(host string) error {
	address := normalizeTemporalHost(host)
	dialer := net.Dialer{Timeout: temporalHealthCheckTimeout}
	connection, dialError := dialer.Dial("tcp", address)
	if dialError != nil {
		return dialError
	}
	if err := connection.Close(); err != nil {
		return err
	}
	return nil
}

func normalizeTemporalHost(host string) string {
	address := strings.TrimSpace(host)
	if address == "" {
		return temporalDefaultHost
	}
	return address
}

func usesStateRoot(dir, root string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	cleanDir := filepath.Clean(dir)
	cleanRoot := filepath.Clean(root)
	if cleanDir == cleanRoot {
		return true
	}
	return strings.HasPrefix(cleanDir, cleanRoot+string(os.PathSeparator))
}

func findStaticDir() string {
	distPath := filepath.Join("gestalt", "dist")
	if info, err := os.Stat(distPath); err == nil && info.IsDir() {
		return distPath
	}

	return ""
}

func loadAgents(logger *logging.Logger, configFS fs.FS, configRoot string, skillIndex map[string]struct{}) (map[string]agent.Agent, error) {
	loader := agent.Loader{Logger: logger}
	return loader.Load(configFS, path.Join(configRoot, "agents"), path.Join(configRoot, "prompts"), skillIndex)
}

func loadSkills(logger *logging.Logger, configFS fs.FS, configRoot string) (map[string]*skill.Skill, error) {
	loader := skill.Loader{Logger: logger}
	return loader.Load(configFS, path.Join(configRoot, "skills"))
}

func buildSkillIndex(skills map[string]*skill.Skill) map[string]struct{} {
	if len(skills) == 0 {
		return map[string]struct{}{}
	}
	index := make(map[string]struct{}, len(skills))
	for name := range skills {
		index[name] = struct{}{}
	}
	return index
}

func runValidateSkill(args []string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, "usage: gestalt validate-skill <path>")
		return 1
	}

	path := strings.TrimSpace(args[0])
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill path error: %v\n", err)
		return 1
	}

	skillPath := path
	if info.IsDir() {
		skillPath = filepath.Join(path, "SKILL.md")
	}

	entry, err := skill.ParseFile(skillPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skill invalid: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "skill ok: %s\n", entry.Name)
	if entry.Description != "" {
		fmt.Fprintf(os.Stdout, "description: %s\n", entry.Description)
	}
	if strings.TrimSpace(entry.License) == "" {
		fmt.Fprintln(os.Stdout, "note: license is empty")
	}
	if strings.TrimSpace(entry.Compatibility) == "" {
		fmt.Fprintln(os.Stdout, "note: compatibility is empty")
	}
	if len(entry.AllowedTools) == 0 {
		fmt.Fprintln(os.Stdout, "note: allowed_tools not set")
	}

	base := entry.Path
	if strings.TrimSpace(base) == "" {
		base = filepath.Dir(skillPath)
	}
	for _, dir := range []string{"scripts", "references", "assets"} {
		if hasOptionalSkillDir(base, dir) {
			fmt.Fprintf(os.Stdout, "ok: %s/ directory present\n", dir)
		} else {
			fmt.Fprintf(os.Stdout, "note: %s/ directory missing\n", dir)
		}
	}

	return 0
}

func hasOptionalSkillDir(base, name string) bool {
	info, err := os.Stat(filepath.Join(base, name))
	if err != nil {
		return false
	}
	return info.IsDir()
}

type configPaths struct {
	Root       string
	SubDir     string
	ConfigDir  string
	VersionLoc string
}

func prepareConfig(cfg Config, logger *logging.Logger) (configPaths, error) {
	paths, err := resolveConfigPaths(cfg.ConfigDir)
	if err != nil {
		return configPaths{}, err
	}
	if cfg.DevMode {
		info, err := os.Stat(paths.ConfigDir)
		if err != nil {
			if os.IsNotExist(err) {
				return configPaths{}, fmt.Errorf("dev mode config dir missing: %s", paths.ConfigDir)
			}
			return configPaths{}, fmt.Errorf("stat dev config dir: %w", err)
		}
		if !info.IsDir() {
			return configPaths{}, fmt.Errorf("dev mode config path is not a directory: %s", paths.ConfigDir)
		}
		if logger != nil {
			logger.Warn("dev mode enabled, skipping config extraction", map[string]string{
				"config_dir": paths.ConfigDir,
			})
		}
		return paths, nil
	}
	if err := os.MkdirAll(paths.ConfigDir, 0o755); err != nil {
		return configPaths{}, fmt.Errorf("create config dir: %w", err)
	}

	current := version.GetVersionInfo()
	installed, err := config.LoadVersionFile(paths.VersionLoc)
	if err != nil && !errors.Is(err, config.ErrVersionFileMissing) {
		return configPaths{}, fmt.Errorf("load version file: %w", err)
	}
	hadInstalled := err == nil
	var lastVersionWrite time.Time
	if err == nil {
		if info, statErr := os.Stat(paths.VersionLoc); statErr == nil {
			lastVersionWrite = info.ModTime()
		}
	}
	if err == nil {
		if compatibilityErr := config.CheckVersionCompatibility(installed, current, logger); compatibilityErr != nil {
			if cfg.ForceUpgrade {
				if logger != nil {
					logger.Warn("config version check overridden by --force-upgrade", map[string]string{
						"error": compatibilityErr.Error(),
					})
				}
			} else {
				return configPaths{}, compatibilityErr
			}
		}
	}

	manifest, err := config.LoadManifest(gestalt.EmbeddedConfigFS)
	if err != nil {
		if errors.Is(err, config.ErrManifestMissing) {
			if logger != nil {
				logger.Warn("config manifest missing, computing hashes at startup", nil)
			}
			manifest, err = buildManifestFromFS(gestalt.EmbeddedConfigFS)
		}
		if err != nil {
			return configPaths{}, fmt.Errorf("load manifest: %w", err)
		}
	}

	extractor := config.Extractor{
		Logger:      logger,
		BackupLimit: cfg.ConfigBackupLimit,
		LastUpdated: lastVersionWrite,
	}
	start := time.Now()
	stats, err := extractor.ExtractWithStats(gestalt.EmbeddedConfigFS, paths.ConfigDir, manifest)
	duration := time.Since(start)
	if err != nil {
		logConfigMetrics(logger, stats, duration, false, err)
		return configPaths{}, err
	}
	if logger != nil {
		logger.Debug("config extraction duration", map[string]string{
			"duration_ms": strconv.FormatInt(duration.Milliseconds(), 10),
		})
	}
	logConfigMetrics(logger, stats, duration, true, nil)
	if err := config.WriteVersionFile(paths.VersionLoc, current); err != nil {
		return configPaths{}, fmt.Errorf("write version file: %w", err)
	}

	logConfigSummary(logger, paths, installed, current, stats, hadInstalled)
	return paths, nil
}

func preparePlanFile(logger *logging.Logger) string {
	targetPath := plan.DefaultPath()
	legacyPath := "PLAN.org"

	legacyInfo, err := os.Stat(legacyPath)
	if err != nil || legacyInfo.IsDir() {
		return targetPath
	}

	if _, err := os.Stat(targetPath); err == nil {
		if logger != nil {
			logger.Warn("PLAN.org exists in multiple locations; using .gestalt/PLAN.org", map[string]string{
				"path": targetPath,
			})
		}
		return targetPath
	} else if !os.IsNotExist(err) {
		if logger != nil {
			logger.Warn("plan path check failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		if logger != nil {
			logger.Warn("plan migration failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}
	if err := copyFile(legacyPath, targetPath); err != nil {
		if logger != nil {
			logger.Warn("plan migration failed", map[string]string{
				"path":  targetPath,
				"error": err.Error(),
			})
		}
		return targetPath
	}
	if logger != nil {
		logger.Info("Migrated PLAN.org to .gestalt/PLAN.org", map[string]string{
			"path": targetPath,
		})
	}
	return targetPath
}

func resolveConfigPaths(configDir string) (configPaths, error) {
	cleaned := filepath.Clean(configDir)
	if strings.TrimSpace(cleaned) == "" {
		return configPaths{}, fmt.Errorf("config dir cannot be empty")
	}
	root := filepath.Dir(cleaned)
	subDir := filepath.Base(cleaned)
	return configPaths{
		Root:       root,
		SubDir:     filepath.ToSlash(subDir),
		ConfigDir:  cleaned,
		VersionLoc: filepath.Join(root, "version.json"),
	}, nil
}

func buildConfigFS(configRoot string) fs.FS {
	return os.DirFS(configRoot)
}

func buildManifestFromFS(sourceFS fs.FS) (map[string]string, error) {
	manifest := make(map[string]string)
	if err := fs.WalkDir(sourceFS, "config", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if path == "config/manifest.json" {
			return nil
		}
		data, err := fs.ReadFile(sourceFS, path)
		if err != nil {
			return err
		}
		hasher := fnv.New64a()
		_, _ = hasher.Write(data)
		relative := strings.TrimPrefix(path, "config/")
		manifest[relative] = fmt.Sprintf("%016x", hasher.Sum64())
		return nil
	}); err != nil {
		return nil, err
	}
	return manifest, nil
}

func logConfigSummary(logger *logging.Logger, paths configPaths, installed, current version.VersionInfo, stats config.ExtractStats, hadInstalled bool) {
	if logger == nil {
		return
	}
	fields := map[string]string{
		"config_dir": paths.ConfigDir,
		"current":    formatVersionInfo(current),
		"extracted":  strconv.Itoa(stats.Extracted),
		"skipped":    strconv.Itoa(stats.Skipped),
		"backed_up":  strconv.Itoa(stats.BackedUp),
	}
	if hadInstalled {
		fields["installed"] = formatVersionInfo(installed)
	} else {
		fields["installed"] = "none"
	}
	logger.Info("config extraction complete", fields)
}

func logConfigMetrics(logger *logging.Logger, stats config.ExtractStats, duration time.Duration, success bool, err error) {
	if logger == nil {
		return
	}
	fields := map[string]string{
		"extracted":   strconv.Itoa(stats.Extracted),
		"skipped":     strconv.Itoa(stats.Skipped),
		"backed_up":   strconv.Itoa(stats.BackedUp),
		"duration_ms": strconv.FormatInt(duration.Milliseconds(), 10),
		"success":     strconv.FormatBool(success),
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	if success {
		logger.Info("config extraction metrics", fields)
		return
	}
	logger.Warn("config extraction metrics", fields)
}

func copyFile(source, destination string) error {
	payload, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(source); err == nil {
		if mode := info.Mode().Perm(); mode != 0 {
			perm = mode
		}
	}
	return os.WriteFile(destination, payload, perm)
}

func formatVersionInfo(info version.VersionInfo) string {
	if strings.TrimSpace(info.Version) != "" {
		return info.Version
	}
	return fmt.Sprintf("%d.%d.%d", info.Major, info.Minor, info.Patch)
}
