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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gestalt"
	"gestalt/internal/agent"
	"gestalt/internal/api"
	"gestalt/internal/logging"
	"gestalt/internal/skill"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
	"gestalt/internal/watcher"

	"github.com/fsnotify/fsnotify"
)

type Config struct {
	Port                 int
	Shell                string
	AuthToken            string
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

const temporalDevServerHost = "localhost:7233"
const temporalDevServerTimeout = 500 * time.Millisecond

type configDefaults struct {
	Port                 int
	Shell                string
	AuthToken            string
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
	logTemporalDevServerHealth(logger)

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
		SessionLogDir:        cfg.SessionLogDir,
		InputHistoryDir:      cfg.InputHistoryDir,
		SessionRetentionDays: cfg.SessionRetentionDays,
		BufferLines:          cfg.SessionBufferLines,
		PromptFS:             configFS,
		PromptDir:            path.Join("config", "prompts"),
	})

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
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server stopped", map[string]string{
			"error": err.Error(),
		})
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

func logTemporalDevServerHealth(logger *logging.Logger) {
	if logger == nil {
		return
	}
	address := temporalDevServerHost
	dialer := net.Dialer{Timeout: temporalDevServerTimeout}
	connection, dialError := dialer.Dial("tcp", address)
	if dialError != nil {
		logger.Warn("temporal server unavailable", map[string]string{
			"host":  address,
			"error": dialError.Error(),
		})
		return
	}
	closeError := connection.Close()
	if closeError != nil {
		logger.Warn("temporal server connection close failed", map[string]string{
			"host":  address,
			"error": closeError.Error(),
		})
		return
	}
	logger.Info("temporal server reachable", map[string]string{
		"host": address,
	})
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
