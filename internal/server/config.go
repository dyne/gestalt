package server

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gestalt/internal/terminal"
)

type Config struct {
	FrontendPort         int
	BackendPort          int
	Shell                string
	AuthToken            string
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

type configDefaults struct {
	FrontendPort         int
	BackendPort          int
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

func LoadConfig(args []string) (Config, error) {
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
	fs.Visit(func(flagValue *flag.Flag) {
		set[flagValue.Name] = true
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
