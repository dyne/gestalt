package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
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
	"gestalt/internal/logging"
	"gestalt/internal/skill"
	"gestalt/internal/temporal"
	temporalworker "gestalt/internal/temporal/worker"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
	temporalconfig "go.temporal.io/server/common/config"
	"go.temporal.io/server/common/dynamicconfig"
	_ "go.temporal.io/server/common/persistence/sql/sqlplugin/sqlite"
	temporalsrv "go.temporal.io/server/temporal"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port                 int
	Shell                string
	AuthToken            string
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
	ShowVersion          bool
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
	Port                 int
	Shell                string
	AuthToken            string
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
}

type flagValues struct {
	Port                 int
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
	ensureStateDir(cfg, logger)

	temporalDevServer, devServerError := startTemporalDevServer(cfg, logger)
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

	configFS := buildConfigFS(logger)
	skills, err := loadSkills(logger, configFS)
	if err != nil {
		logger.Error("load skills failed", map[string]string{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	logger.Info("skills loaded", map[string]string{
		"count": strconv.Itoa(len(skills)),
	})

	agents, err := loadAgents(logger, configFS, buildSkillIndex(skills))
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
		PromptDir:            path.Join("config", "prompts"),
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

	fsWatcher, err := watcher.NewWithOptions(watcher.Options{
		Logger:     logger,
		MaxWatches: cfg.MaxWatches,
	})
	if err != nil && logger != nil {
		logger.Warn("filesystem watcher unavailable", map[string]string{
			"error": err.Error(),
		})
	}
	eventHub := watcher.NewEventHub(context.Background(), fsWatcher)
	if fsWatcher != nil {
		fsWatcher.SetErrorHandler(func(err error) {
			eventHub.Publish(watcher.Event{
				Type:      watcher.EventTypeWatchError,
				Timestamp: time.Now().UTC(),
			})
		})
		watchPlanFile(eventHub, logger, "PLAN.org")
		if workDir, err := os.Getwd(); err == nil {
			if _, err := watcher.StartGitWatcher(eventHub, workDir); err != nil && logger != nil {
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

	mux := http.NewServeMux()
	api.RegisterRoutes(mux, manager, cfg.AuthToken, staticDir, frontendFS, logger, eventHub)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("gestalt listening", map[string]string{
		"addr":    server.Addr,
		"version": version.Version,
	})

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", map[string]string{
				"error": err.Error(),
			})
		}
	case sig := <-stopSignals:
		logger.Info("shutdown signal received", map[string]string{
			"signal": sig.String(),
		})
		shutdownContext, cancel := context.WithTimeout(context.Background(), httpServerShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Warn("http server shutdown failed", map[string]string{
				"error": err.Error(),
			})
		}
		<-serverErrors
	}
}

func watchPlanFile(eventHub *watcher.EventHub, logger *logging.Logger, planPath string) {
	if eventHub == nil {
		return
	}
	if planPath == "" {
		planPath = "PLAN.org"
	}

	var retryMutex sync.Mutex
	retrying := false

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
				if err := eventHub.WatchFile(planPath); err == nil {
					if logger != nil {
						logger.Info("Watching PLAN.org for changes", map[string]string{
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

	if err := eventHub.WatchFile(planPath); err != nil {
		if logger != nil {
			logger.Warn("plan watch failed", map[string]string{
				"path":  planPath,
				"error": err.Error(),
			})
		}
		startRetry()
	} else if logger != nil {
		logger.Info("Watching PLAN.org for changes", map[string]string{
			"path": planPath,
		})
	}

	eventHub.Subscribe(watcher.EventTypeFileChanged, func(event watcher.Event) {
		if event.Path != planPath {
			return
		}
		if event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 {
			return
		}
		_ = eventHub.UnwatchFile(planPath)
		startRetry()
	})
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

	port := defaults.Port
	portSource := sourceDefault
	if rawPort := os.Getenv("GESTALT_PORT"); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 {
			port = parsed
			portSource = sourceEnv
		}
	}
	if flags.Set["port"] {
		if flags.Port <= 0 {
			return Config{}, fmt.Errorf("invalid --port: must be > 0")
		}
		port = flags.Port
		portSource = sourceFlag
	}
	cfg.Port = port
	cfg.Sources["port"] = portSource

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

	return cfg, nil
}

func defaultConfigValues() configDefaults {
	return configDefaults{
		Port:                 8080,
		Shell:                terminal.DefaultShell(),
		AuthToken:            "",
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
		MaxWatches:           100,
	}
}

func parseFlags(args []string, defaults configDefaults) (flagValues, error) {
	if args == nil {
		args = []string{}
	}
	fs := flag.NewFlagSet("gestalt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	port := fs.Int("port", defaults.Port, "HTTP server port")
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
		Port:                 *port,
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
			Desc: fmt.Sprintf("HTTP server port (env: GESTALT_PORT, default: %d)", defaults.Port),
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
		flags = append(flags, fmt.Sprintf("--port %d", cfg.Port))
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

	if len(flags) == 0 {
		return
	}
	logger.Debug("starting with flags", map[string]string{
		"flags": strings.Join(flags, " "),
	})
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
	if !usesStateRoot(cfg.SessionLogDir, stateRoot) && !usesStateRoot(cfg.InputHistoryDir, stateRoot) {
		return
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil && logger != nil {
		logger.Warn("create state dir failed", map[string]string{
			"path":  stateRoot,
			"error": err.Error(),
		})
	}
}

const temporalDevServerConfigYAML = `log:
  stdout: true
  level: info

persistence:
  defaultStore: sqlite-default
  visibilityStore: sqlite-visibility
  numHistoryShards: 1
  datastores:
    sqlite-default:
      sql:
        user: ""
        password: ""
        pluginName: "sqlite"
        databaseName: "default"
        connectAddr: "localhost"
        connectProtocol: "tcp"
        connectAttributes:
          cache: "private"
          setup: true
          journal_mode: wal
          synchronous: 2
        maxConns: 1
        maxIdleConns: 1
        maxConnLifetime: "1h"
        tls:
          enabled: false
          caFile: ""
          certFile: ""
          keyFile: ""
          enableHostVerification: false
          serverName: ""

    sqlite-visibility:
      sql:
        user: ""
        password: ""
        pluginName: "sqlite"
        databaseName: "default"
        connectAddr: "localhost"
        connectProtocol: "tcp"
        connectAttributes:
          cache: "private"
          setup: true
          journal_mode: wal
          synchronous: 2
        maxConns: 1
        maxIdleConns: 1
        maxConnLifetime: "1h"
        tls:
          enabled: false
          caFile: ""
          certFile: ""
          keyFile: ""
          enableHostVerification: false
          serverName: ""
global:
  membership:
    maxJoinDuration: 30s
    broadcastAddress: "127.0.0.1"
  pprof:
    port: 7936
  metrics:
    prometheus:
      framework: "tally"
      timerType: "histogram"
      listenAddress: "127.0.0.1:8000"

services:
  frontend:
    rpc:
      grpcPort: 7233
      membershipPort: 6933
      bindOnLocalHost: true
      httpPort: 7243

  matching:
    rpc:
      grpcPort: 7235
      membershipPort: 6935
      bindOnLocalHost: true

  history:
    rpc:
      grpcPort: 7234
      membershipPort: 6934
      bindOnLocalHost: true

  worker:
    rpc:
      grpcPort: 7239
      membershipPort: 6939
      bindOnLocalHost: true

clusterMetadata:
  enableGlobalNamespace: false
  failoverVersionIncrement: 10
  masterClusterName: "active"
  currentClusterName: "active"
  clusterInformation:
    active:
      enabled: true
      initialFailoverVersion: 1
      rpcName: "frontend"
      rpcAddress: "localhost:7233"
      httpAddress: "localhost:7243"

dcRedirectionPolicy:
  policy: "noop"

archival:
  history:
    state: "enabled"
    enableRead: true
    provider:
      filestore:
        fileMode: "0666"
        dirMode: "0766"
      gstorage:
        credentialsPath: "/tmp/gcloud/keyfile.json"
  visibility:
    state: "enabled"
    enableRead: true
    provider:
      filestore:
        fileMode: "0666"
        dirMode: "0766"

namespaceDefaults:
  archival:
    history:
      state: "disabled"
      URI: "file:///tmp/temporal_archival/development"
    visibility:
      state: "disabled"
      URI: "file:///tmp/temporal_vis_archival/development"

dynamicConfigClient:
  filepath: "config/dynamicconfig/development-sql.yaml"
  pollInterval: "10s"
`

const temporalDevServerDynamicConfigYAML = `system.enableEagerWorkflowStart:
  - value: true
limit.maxIDLength:
  - value: 255
    constraints: {}
frontend.workerVersioningDataAPIs:
  - value: true
frontend.workerVersioningWorkflowAPIs:
  - value: true
frontend.workerVersioningRuleAPIs:
  - value: true
system.enableDeploymentVersions:
  - value: true
system.enableDeployments:
  - value: true
frontend.enableExecuteMultiOperation:
  - value: true
system.enableNexus:
  - value: true
component.nexusoperations.callback.endpoint.template:
  - value: http://localhost:7243/namespaces/{{.NamespaceName}}/nexus/callback
component.callbacks.allowedAddresses:
  - value:
      - Pattern: "*"
        AllowInsecure: true
matching.queryWorkflowTaskTimeoutLogRate:
  - value: 1.0
history.ReplicationEnableUpdateWithNewTaskMerge:
  - value: true
history.enableWorkflowExecutionTimeoutTimer:
  - value: true
history.hostLevelCacheMaxSize:
  - value: 8192
`

type temporalDevConfigPaths struct {
	LogPath               string
	DatabasePath          string
	DynamicConfigPath     string
	ArchivalHistoryURI    string
	ArchivalVisibilityURI string
}

func buildTemporalDevConfig(paths temporalDevConfigPaths) (*temporalconfig.Config, error) {
	if strings.TrimSpace(paths.LogPath) == "" {
		return nil, errors.New("temporal log path is required")
	}
	if strings.TrimSpace(paths.DatabasePath) == "" {
		return nil, errors.New("temporal database path is required")
	}
	if strings.TrimSpace(paths.DynamicConfigPath) == "" {
		return nil, errors.New("temporal dynamic config path is required")
	}

	var serverConfig temporalconfig.Config
	if err := yaml.Unmarshal([]byte(temporalDevServerConfigYAML), &serverConfig); err != nil {
		return nil, fmt.Errorf("parse temporal config: %w", err)
	}

	if serverConfig.DynamicConfigClient == nil {
		serverConfig.DynamicConfigClient = &dynamicconfig.FileBasedClientConfig{}
	}
	serverConfig.DynamicConfigClient.Filepath = paths.DynamicConfigPath
	serverConfig.Log.Stdout = false
	serverConfig.Log.OutputFile = paths.LogPath

	if err := updateTemporalSQLiteDatastore(&serverConfig, "sqlite-default", paths.DatabasePath); err != nil {
		return nil, err
	}
	if err := updateTemporalSQLiteDatastore(&serverConfig, "sqlite-visibility", paths.DatabasePath); err != nil {
		return nil, err
	}

	if strings.TrimSpace(paths.ArchivalHistoryURI) != "" {
		serverConfig.NamespaceDefaults.Archival.History.URI = paths.ArchivalHistoryURI
	}
	if strings.TrimSpace(paths.ArchivalVisibilityURI) != "" {
		serverConfig.NamespaceDefaults.Archival.Visibility.URI = paths.ArchivalVisibilityURI
	}

	if err := serverConfig.Validate(); err != nil {
		return nil, fmt.Errorf("validate temporal config: %w", err)
	}

	return &serverConfig, nil
}

func updateTemporalSQLiteDatastore(cfg *temporalconfig.Config, storeName, databasePath string) error {
	if cfg == nil {
		return errors.New("temporal config is required")
	}
	if strings.TrimSpace(databasePath) == "" {
		return fmt.Errorf("temporal database path is required for %s", storeName)
	}
	datastore, ok := cfg.Persistence.DataStores[storeName]
	if !ok {
		return fmt.Errorf("temporal datastore %q not found", storeName)
	}
	if datastore.SQL == nil {
		return fmt.Errorf("temporal datastore %q missing sql config", storeName)
	}
	datastore.SQL.DatabaseName = databasePath
	cfg.Persistence.DataStores[storeName] = datastore
	return nil
}

func writeTemporalDynamicConfig(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("temporal dynamic config path is required")
	}
	return os.WriteFile(path, []byte(temporalDevServerDynamicConfigYAML), 0o644)
}

type temporalDevServer struct {
	server   temporalsrv.Server
	done     chan error
	stopOnce sync.Once
}

func startTemporalDevServer(cfg Config, logger *logging.Logger) (*temporalDevServer, error) {
	if !cfg.TemporalDevServer {
		return nil, nil
	}

	dataDir := filepath.Join(".gestalt", "temporal")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		absDataDir = dataDir
	}
	configDir := filepath.Join(absDataDir, "config")
	dynamicConfigDir := filepath.Join(configDir, "dynamicconfig")
	historyArchivalDir := filepath.Join(absDataDir, "archival", "history")
	visibilityArchivalDir := filepath.Join(absDataDir, "archival", "visibility")
	if err := os.MkdirAll(dynamicConfigDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal config dir: %w", err)
	}
	if err := os.MkdirAll(historyArchivalDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal history archival dir: %w", err)
	}
	if err := os.MkdirAll(visibilityArchivalDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporal visibility archival dir: %w", err)
	}

	logPath := filepath.Join(absDataDir, "temporal.log")
	databasePath := filepath.Join(absDataDir, "temporal.db")
	dynamicConfigPath := filepath.Join(dynamicConfigDir, "development-sql.yaml")
	if err := writeTemporalDynamicConfig(dynamicConfigPath); err != nil {
		return nil, fmt.Errorf("write temporal dynamic config: %w", err)
	}
	archivalHistoryURI := "file://" + filepath.ToSlash(historyArchivalDir)
	archivalVisibilityURI := "file://" + filepath.ToSlash(visibilityArchivalDir)
	serverConfig, err := buildTemporalDevConfig(temporalDevConfigPaths{
		LogPath:               logPath,
		DatabasePath:          databasePath,
		DynamicConfigPath:     dynamicConfigPath,
		ArchivalHistoryURI:    archivalHistoryURI,
		ArchivalVisibilityURI: archivalVisibilityURI,
	})
	if err != nil {
		return nil, err
	}
	server, err := temporalsrv.NewServer(
		temporalsrv.WithConfig(serverConfig),
		temporalsrv.ForServices(temporalsrv.DefaultServices),
	)
	if err != nil {
		return nil, fmt.Errorf("create temporal server: %w", err)
	}
	if err := server.Start(); err != nil {
		return nil, fmt.Errorf("start temporal server: %w", err)
	}

	if logger != nil {
		logger.Info("temporal dev server started", map[string]string{
			"dir": absDataDir,
			"log": logPath,
			"db":  databasePath,
		})
	}

	return &temporalDevServer{
		server: server,
		done:   make(chan error, 1),
	}, nil
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
	server.stopOnce.Do(func() {
		stopResult := make(chan error, 1)
		go func() {
			if server.server == nil {
				stopResult <- nil
				return
			}
			stopResult <- server.server.Stop()
		}()

		var stopErr error
		select {
		case stopErr = <-stopResult:
		case <-time.After(temporalDevServerStopTimeout):
			stopErr = fmt.Errorf("timed out after %s", temporalDevServerStopTimeout)
		}

		if stopErr != nil && logger != nil {
			logger.Warn("temporal dev server stop failed", map[string]string{
				"error": stopErr.Error(),
			})
		}
		if server.done != nil {
			if stopErr != nil {
				select {
				case server.done <- stopErr:
				default:
				}
			}
			close(server.done)
		}
	})
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

func loadAgents(logger *logging.Logger, configFS fs.FS, skillIndex map[string]struct{}) (map[string]agent.Agent, error) {
	loader := agent.Loader{Logger: logger}
	return loader.Load(configFS, path.Join("config", "agents"), path.Join("config", "prompts"), skillIndex)
}

func loadSkills(logger *logging.Logger, configFS fs.FS) (map[string]*skill.Skill, error) {
	loader := skill.Loader{Logger: logger}
	return loader.Load(configFS, path.Join("config", "skills"))
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

func buildConfigFS(logger *logging.Logger) fs.FS {
	overrideRoot := "gestalt"
	configDir := filepath.Join(overrideRoot, "config")
	useExternal := map[string]bool{
		"agents":  dirExists(filepath.Join(configDir, "agents")),
		"prompts": dirExists(filepath.Join(configDir, "prompts")),
		"skills":  dirExists(filepath.Join(configDir, "skills")),
	}

	if logger != nil {
		if useExternal["agents"] || useExternal["prompts"] || useExternal["skills"] {
			logger.Info("using external config at ./gestalt/", map[string]string{
				"agents":  strconv.FormatBool(useExternal["agents"]),
				"prompts": strconv.FormatBool(useExternal["prompts"]),
				"skills":  strconv.FormatBool(useExternal["skills"]),
			})
		} else {
			logger.Info("using embedded config", nil)
		}
	}

	return configFS{
		embedded:     gestalt.EmbeddedConfigFS,
		external:     os.DirFS("."),
		externalRoot: filepath.ToSlash(overrideRoot),
		useExternal:  useExternal,
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
