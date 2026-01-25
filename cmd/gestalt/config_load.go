package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt"
	"gestalt/internal/cli"
	"gestalt/internal/config"
	"gestalt/internal/logging"
	"gestalt/internal/plan"
	"gestalt/internal/skill"
	"gestalt/internal/terminal"
	"gestalt/internal/version"
)

type Config struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	AuthToken            string
	SCIPIndexPath        string
	SCIPAutoReindex      bool
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
	PprofEnabled         bool
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

type configDefaults struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	AuthToken            string
	SCIPIndexPath        string
	SCIPAutoReindex      bool
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
	PprofEnabled         bool
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
	SCIPIndexPath        string
	SCIPAutoReindex      bool
	ConfigDir            string
	ConfigBackupLimit    int
	MaxWatches           int
	PprofEnabled         bool
	Verbose              bool
	Quiet                bool
	Help                 bool
	Version              bool
	ForceUpgrade         bool
	DevMode              bool
	Set                  map[string]bool
}

type helpOption struct {
	Name string
	Desc string
}

type configPaths struct {
	Root       string
	SubDir     string
	ConfigDir  string
	VersionLoc string
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

	temporalHost := defaults.TemporalHost
	temporalHostSource := sourceDefault
	if rawHost := strings.TrimSpace(os.Getenv("GESTALT_TEMPORAL_HOST")); rawHost != "" {
		temporalHost = rawHost
		temporalHostSource = sourceEnv
	}
	if flags.Set["temporal-host"] {
		temporalHost = flags.TemporalHost
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
		temporalNamespace = flags.TemporalNamespace
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

	sessionPersist := defaults.SessionPersist
	sessionPersistSource := sourceDefault
	if rawPersist := strings.TrimSpace(os.Getenv("GESTALT_SESSION_PERSIST")); rawPersist != "" {
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

	sessionLogDir := defaults.SessionLogDir
	sessionLogDirSource := sourceDefault
	if rawSessionDir := strings.TrimSpace(os.Getenv("GESTALT_SESSION_DIR")); rawSessionDir != "" {
		sessionLogDir = rawSessionDir
		sessionLogDirSource = sourceEnv
	}
	if flags.Set["session-dir"] {
		trimmed := strings.TrimSpace(flags.SessionLogDir)
		if trimmed == "" {
			return Config{}, fmt.Errorf("invalid --session-dir: value cannot be empty")
		}
		sessionLogDir = trimmed
		sessionLogDirSource = sourceFlag
	}
	cfg.SessionLogDir = sessionLogDir
	cfg.Sources["session-dir"] = sessionLogDirSource

	sessionRetention := defaults.SessionRetentionDays
	sessionRetentionSource := sourceDefault
	if rawRetention := strings.TrimSpace(os.Getenv("GESTALT_SESSION_RETENTION_DAYS")); rawRetention != "" {
		if parsed, err := strconv.Atoi(rawRetention); err == nil && parsed > 0 {
			sessionRetention = parsed
			sessionRetentionSource = sourceEnv
		}
	}
	if flags.Set["session-retention-days"] {
		if flags.SessionRetentionDays <= 0 {
			return Config{}, fmt.Errorf("invalid --session-retention-days: must be > 0")
		}
		sessionRetention = flags.SessionRetentionDays
		sessionRetentionSource = sourceFlag
	}
	cfg.SessionRetentionDays = sessionRetention
	cfg.Sources["session-retention-days"] = sessionRetentionSource

	sessionBufferLines := defaults.SessionBufferLines
	sessionBufferLinesSource := sourceDefault
	if rawBuffer := strings.TrimSpace(os.Getenv("GESTALT_SESSION_BUFFER_LINES")); rawBuffer != "" {
		if parsed, err := strconv.Atoi(rawBuffer); err == nil && parsed > 0 {
			sessionBufferLines = parsed
			sessionBufferLinesSource = sourceEnv
		}
	}
	if flags.Set["session-buffer-lines"] {
		if flags.SessionBufferLines <= 0 {
			return Config{}, fmt.Errorf("invalid --session-buffer-lines: must be > 0")
		}
		sessionBufferLines = flags.SessionBufferLines
		sessionBufferLinesSource = sourceFlag
	}
	cfg.SessionBufferLines = sessionBufferLines
	cfg.Sources["session-buffer-lines"] = sessionBufferLinesSource

	historyPersist := defaults.InputHistoryPersist
	historyPersistSource := sourceDefault
	if rawPersist := strings.TrimSpace(os.Getenv("GESTALT_INPUT_HISTORY_PERSIST")); rawPersist != "" {
		if parsed, err := strconv.ParseBool(rawPersist); err == nil {
			historyPersist = parsed
			historyPersistSource = sourceEnv
		}
	}
	if flags.Set["input-history-persist"] {
		historyPersist = flags.InputHistoryPersist
		historyPersistSource = sourceFlag
	}
	cfg.InputHistoryPersist = historyPersist
	cfg.Sources["input-history-persist"] = historyPersistSource

	historyDir := defaults.InputHistoryDir
	historyDirSource := sourceDefault
	if rawDir := strings.TrimSpace(os.Getenv("GESTALT_INPUT_HISTORY_DIR")); rawDir != "" {
		historyDir = rawDir
		historyDirSource = sourceEnv
	}
	if flags.Set["input-history-dir"] {
		trimmed := strings.TrimSpace(flags.InputHistoryDir)
		if trimmed == "" {
			return Config{}, fmt.Errorf("invalid --input-history-dir: value cannot be empty")
		}
		historyDir = trimmed
		historyDirSource = sourceFlag
	}
	cfg.InputHistoryDir = historyDir
	cfg.Sources["input-history-dir"] = historyDirSource

	scipIndex := defaults.SCIPIndexPath
	scipIndexSource := sourceDefault
	if rawIndex := strings.TrimSpace(os.Getenv("GESTALT_SCIP_INDEX_PATH")); rawIndex != "" {
		scipIndex = rawIndex
		scipIndexSource = sourceEnv
	}
	if flags.Set["scip-index-path"] {
		scipIndex = flags.SCIPIndexPath
		scipIndexSource = sourceFlag
	}
	cfg.SCIPIndexPath = scipIndex
	cfg.Sources["scip-index-path"] = scipIndexSource

	scipAutoReindex := defaults.SCIPAutoReindex
	scipAutoReindexSource := sourceDefault
	if rawAuto := strings.TrimSpace(os.Getenv("GESTALT_SCIP_AUTO_REINDEX")); rawAuto != "" {
		if parsed, err := strconv.ParseBool(rawAuto); err == nil {
			scipAutoReindex = parsed
			scipAutoReindexSource = sourceEnv
		}
	}
	if flags.Set["scip-auto-reindex"] {
		scipAutoReindex = flags.SCIPAutoReindex
		scipAutoReindexSource = sourceFlag
	}
	cfg.SCIPAutoReindex = scipAutoReindex
	cfg.Sources["scip-auto-reindex"] = scipAutoReindexSource

	configDir := defaults.ConfigDir
	configDirSource := sourceDefault
	if rawDir := strings.TrimSpace(os.Getenv("GESTALT_CONFIG_DIR")); rawDir != "" {
		configDir = rawDir
		configDirSource = sourceEnv
	}
	if flags.Set["config-dir"] {
		configDir = flags.ConfigDir
		configDirSource = sourceFlag
	}
	cfg.ConfigDir = configDir
	cfg.Sources["config-dir"] = configDirSource

	backupLimit := defaults.ConfigBackupLimit
	backupSource := sourceDefault
	if rawLimit := strings.TrimSpace(os.Getenv("GESTALT_CONFIG_BACKUP_LIMIT")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed >= 0 {
			backupLimit = parsed
			backupSource = sourceEnv
		}
	}
	if flags.Set["config-backup-limit"] {
		if flags.ConfigBackupLimit < 0 {
			return Config{}, fmt.Errorf("invalid --config-backup-limit: must be >= 0")
		}
		backupLimit = flags.ConfigBackupLimit
		backupSource = sourceFlag
	}
	cfg.ConfigBackupLimit = backupLimit
	cfg.Sources["config-backup-limit"] = backupSource

	devModeSource := sourceDefault
	cfg.DevMode = defaults.DevMode
	if rawDevMode := strings.TrimSpace(os.Getenv("GESTALT_DEV_MODE")); rawDevMode != "" {
		if parsed, err := strconv.ParseBool(rawDevMode); err == nil {
			cfg.DevMode = parsed
			devModeSource = sourceEnv
		}
	}
	if flags.Set["dev"] {
		cfg.DevMode = flags.DevMode
		devModeSource = sourceFlag
	}
	cfg.Sources["dev"] = devModeSource

	if cfg.DevMode && cfg.Sources["config-dir"] == sourceDefault {
		cfg.ConfigDir = "config"
	}

	if !cfg.SessionPersist {
		cfg.SessionLogDir = ""
	}
	if !cfg.InputHistoryPersist {
		cfg.InputHistoryDir = ""
	}

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

	pprofEnabled := defaults.PprofEnabled
	pprofSource := sourceDefault
	if rawEnabled := strings.TrimSpace(os.Getenv("GESTALT_PPROF_ENABLED")); rawEnabled != "" {
		if parsed, err := strconv.ParseBool(rawEnabled); err == nil {
			pprofEnabled = parsed
			pprofSource = sourceEnv
		}
	}
	if flags.Set["pprof"] {
		pprofEnabled = flags.PprofEnabled
		pprofSource = sourceFlag
	}
	cfg.PprofEnabled = pprofEnabled
	cfg.Sources["pprof"] = pprofSource

	verboseSource := sourceDefault
	cfg.Verbose = flags.Verbose
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
		SCIPIndexPath:        filepath.Join(".gestalt", "scip", "index.db"),
		SCIPAutoReindex:      false,
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
		PprofEnabled:         false,
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
	scipIndexPath := fs.String("scip-index-path", defaults.SCIPIndexPath, "SCIP index path")
	scipAutoReindex := fs.Bool("scip-auto-reindex", defaults.SCIPAutoReindex, "Auto-reindex SCIP on file changes")
	configDir := fs.String("config-dir", defaults.ConfigDir, "Config directory")
	configBackupLimit := fs.Int("config-backup-limit", defaults.ConfigBackupLimit, "Config backup limit")
	maxWatches := fs.Int("max-watches", defaults.MaxWatches, "Max active watches")
	pprofEnabled := fs.Bool("pprof", defaults.PprofEnabled, "Enable pprof debug endpoints")
	forceUpgrade := fs.Bool("force-upgrade", defaults.ForceUpgrade, "Bypass config version compatibility checks")
	devMode := fs.Bool("dev", defaults.DevMode, "Enable developer mode (skip config extraction)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	quiet := fs.Bool("quiet", false, "Reduce logging to warnings")
	helpVersion := cli.AddHelpVersionFlags(fs, "Show help", "Print version and exit")

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
		SCIPIndexPath:        *scipIndexPath,
		SCIPAutoReindex:      *scipAutoReindex,
		ConfigDir:            *configDir,
		ConfigBackupLimit:    *configBackupLimit,
		MaxWatches:           *maxWatches,
		PprofEnabled:         *pprofEnabled,
		ForceUpgrade:         *forceUpgrade,
		DevMode:              *devMode,
		Verbose:              *verbose,
		Quiet:                *quiet,
		Help:                 helpVersion.Help,
		Version:              helpVersion.Version,
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
		{
			Name: "--pprof",
			Desc: fmt.Sprintf("Enable pprof endpoints (env: GESTALT_PPROF_ENABLED, default: %t)", defaults.PprofEnabled),
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
			Name: "--session-retention-days N",
			Desc: fmt.Sprintf("Session retention days (env: GESTALT_SESSION_RETENTION_DAYS, default: %d)", defaults.SessionRetentionDays),
		},
		{
			Name: "--input-history-persist",
			Desc: fmt.Sprintf("Persist input history (env: GESTALT_INPUT_HISTORY_PERSIST, default: %t)", defaults.InputHistoryPersist),
		},
		{
			Name: "--input-history-dir DIR",
			Desc: fmt.Sprintf("Input history directory (env: GESTALT_INPUT_HISTORY_DIR, default: %s)", defaults.InputHistoryDir),
		},
	})

	writeOptionGroup(out, "Config", []helpOption{
		{
			Name: "--config-dir DIR",
			Desc: fmt.Sprintf("Config directory (env: GESTALT_CONFIG_DIR, default: %s)", defaults.ConfigDir),
		},
		{
			Name: "--config-backup-limit N",
			Desc: fmt.Sprintf("Config backup limit (env: GESTALT_CONFIG_BACKUP_LIMIT, default: %d)", defaults.ConfigBackupLimit),
		},
		{
			Name: "--dev",
			Desc: fmt.Sprintf("Skip config extraction (env: GESTALT_DEV_MODE, default: %t)", defaults.DevMode),
		},
		{
			Name: "--force-upgrade",
			Desc: fmt.Sprintf("Bypass version checks (env: GESTALT_FORCE_UPGRADE, default: %t)", defaults.ForceUpgrade),
		},
	})

	writeOptionGroup(out, "SCIP", []helpOption{
		{
			Name: "--scip-index-path PATH",
			Desc: fmt.Sprintf("SCIP index path (env: GESTALT_SCIP_INDEX_PATH, default: %s)", defaults.SCIPIndexPath),
		},
		{
			Name: "--scip-auto-reindex",
			Desc: fmt.Sprintf("Auto-reindex SCIP on changes (env: GESTALT_SCIP_AUTO_REINDEX, default: %t)", defaults.SCIPAutoReindex),
		},
	})

	writeOptionGroup(out, "Filesystem", []helpOption{
		{
			Name: "--max-watches N",
			Desc: fmt.Sprintf("Max active watches (env: GESTALT_MAX_WATCHES, default: %d)", defaults.MaxWatches),
		},
	})

	writeOptionGroup(out, "Logging", []helpOption{
		{
			Name: "--verbose",
			Desc: "Enable verbose logging",
		},
		{
			Name: "--quiet",
			Desc: "Reduce logging to warnings",
		},
	})

	writeOptionGroup(out, "Other", []helpOption{
		{
			Name: "--help, -h",
			Desc: "Show help and exit",
		},
		{
			Name: "--version, -v",
			Desc: "Print version and exit",
		},
	})
}

func writeOptionGroup(out io.Writer, title string, options []helpOption) {
	if len(options) == 0 {
		return
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, title+":")
	for _, option := range options {
		fmt.Fprintf(out, "  %-24s %s\n", option.Name, option.Desc)
	}
}

func logStartupFlags(logger *logging.Logger, cfg Config) {
	if logger == nil {
		return
	}
	flags := []string{}
	if cfg.Sources["port"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--port", cfg.FrontendPort != 0))
	}
	if cfg.Sources["backend-port"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--backend-port", cfg.BackendPort != 0))
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
	if cfg.Sources["session-retention-days"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--session-retention-days", cfg.SessionRetentionDays != 0))
	}
	if cfg.Sources["session-buffer-lines"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--session-buffer-lines", cfg.SessionBufferLines != 0))
	}
	if cfg.Sources["input-history-persist"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--input-history-persist", cfg.InputHistoryPersist))
	}
	if cfg.Sources["input-history-dir"] == sourceFlag {
		flags = append(flags, formatStringFlag("--input-history-dir", cfg.InputHistoryDir))
	}
	if cfg.Sources["scip-index-path"] == sourceFlag {
		flags = append(flags, formatStringFlag("--scip-index-path", cfg.SCIPIndexPath))
	}
	if cfg.Sources["scip-auto-reindex"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--scip-auto-reindex", cfg.SCIPAutoReindex))
	}
	if cfg.Sources["config-dir"] == sourceFlag {
		flags = append(flags, formatStringFlag("--config-dir", cfg.ConfigDir))
	}
	if cfg.Sources["config-backup-limit"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--config-backup-limit", cfg.ConfigBackupLimit != 0))
	}
	if cfg.Sources["dev"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--dev", cfg.DevMode))
	}
	if cfg.Sources["max-watches"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--max-watches", cfg.MaxWatches != 0))
	}
	if cfg.Sources["pprof"] == sourceFlag {
		flags = append(flags, formatBoolFlag("--pprof", cfg.PprofEnabled))
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
	if len(flags) > 0 {
		logger.Debug("startup flags", map[string]string{
			"flags": strings.Join(flags, " "),
		})
	}
}

func logVersionInfo(logger *logging.Logger) {
	if logger == nil {
		return
	}
	versionLabel := version.Version
	if strings.TrimSpace(versionLabel) == "" {
		versionLabel = "dev"
	}
	logger.Info(fmt.Sprintf("Gestalt version %s", versionLabel), map[string]string{
		"version": versionLabel,
	})
}

func formatBoolFlag(name string, value bool) string {
	if value {
		return name
	}
	return ""
}

func formatStringFlag(name, value string) string {
	if value == "" {
		return ""
	}
	return name + " " + value
}

func formatTokenFlag(token string) string {
	if token == "" {
		return ""
	}
	return "--token ****"
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

	extractor := config.Extractor{
		Logger:      logger,
		BackupLimit: cfg.ConfigBackupLimit,
		LastUpdated: lastVersionWrite,
	}
	start := time.Now()
	stats, err := extractor.ExtractWithStats(gestalt.EmbeddedConfigFS, paths.ConfigDir, nil)
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

func logConfigSummary(logger *logging.Logger, paths configPaths, installed, current version.VersionInfo, stats config.ExtractStats, hadInstalled bool) {
	if logger == nil {
		return
	}
	fields := map[string]string{
		"config_dir": paths.ConfigDir,
		"current":    formatVersionInfo(current),
		"extracted":  strconv.Itoa(stats.Extracted),
		"skipped":    strconv.Itoa(stats.Skipped),
	}
	if hadInstalled {
		fields["previous"] = formatVersionInfo(installed)
	}
	logger.Info("config extraction completed", fields)
}

func logConfigMetrics(logger *logging.Logger, stats config.ExtractStats, duration time.Duration, success bool, err error) {
	if logger == nil {
		return
	}
	fields := map[string]string{
		"success":     strconv.FormatBool(success),
		"duration_ms": strconv.FormatInt(duration.Milliseconds(), 10),
		"extracted":   strconv.Itoa(stats.Extracted),
		"skipped":     strconv.Itoa(stats.Skipped),
		"backed_up":   strconv.Itoa(stats.BackedUp),
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	logger.Info("config extraction metrics", fields)
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

func formatVersionInfo(info version.VersionInfo) string {
	if info.Version == "" {
		return "unknown"
	}
	return info.Version
}
