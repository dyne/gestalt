package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
	"gestalt/internal/version"
)

func LogStartupFlags(logger *logging.Logger, cfg Config) {
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

func LogVersionInfo(logger *logging.Logger) {
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

func EnsureStateDir(cfg Config, logger *logging.Logger) {
	stateRoot := ".gestalt"
	if !usesStateRoot(cfg.SessionLogDir, stateRoot) && !usesStateRoot(cfg.InputHistoryDir, stateRoot) && !usesStateRoot(cfg.ConfigDir, stateRoot) {
		return
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil && logger != nil {
		logger.Warn("create state dir failed", map[string]string{
			"path":  stateRoot,
			"error": err.Error(),
		})
	}
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
