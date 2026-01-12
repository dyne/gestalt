package server

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gestalt"
	"gestalt/internal/config"
	"gestalt/internal/logging"
	"gestalt/internal/version"
)

type ConfigPaths struct {
	Root       string
	SubDir     string
	ConfigDir  string
	VersionLoc string
}

func PrepareConfig(cfg Config, logger *logging.Logger) (ConfigPaths, error) {
	paths, err := resolveConfigPaths(cfg.ConfigDir)
	if err != nil {
		return ConfigPaths{}, err
	}
	if cfg.DevMode {
		info, err := os.Stat(paths.ConfigDir)
		if err != nil {
			if os.IsNotExist(err) {
				return ConfigPaths{}, fmt.Errorf("dev mode config dir missing: %s", paths.ConfigDir)
			}
			return ConfigPaths{}, fmt.Errorf("stat dev config dir: %w", err)
		}
		if !info.IsDir() {
			return ConfigPaths{}, fmt.Errorf("dev mode config path is not a directory: %s", paths.ConfigDir)
		}
		if logger != nil {
			logger.Warn("dev mode enabled, skipping config extraction", map[string]string{
				"config_dir": paths.ConfigDir,
			})
		}
		return paths, nil
	}
	if err := os.MkdirAll(paths.ConfigDir, 0o755); err != nil {
		return ConfigPaths{}, fmt.Errorf("create config dir: %w", err)
	}

	current := version.GetVersionInfo()
	installed, err := config.LoadVersionFile(paths.VersionLoc)
	if err != nil && !errors.Is(err, config.ErrVersionFileMissing) {
		return ConfigPaths{}, fmt.Errorf("load version file: %w", err)
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
				return ConfigPaths{}, compatibilityErr
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
			return ConfigPaths{}, fmt.Errorf("load manifest: %w", err)
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
		return ConfigPaths{}, err
	}
	if logger != nil {
		logger.Debug("config extraction duration", map[string]string{
			"duration_ms": strconv.FormatInt(duration.Milliseconds(), 10),
		})
	}
	logConfigMetrics(logger, stats, duration, true, nil)
	if err := config.WriteVersionFile(paths.VersionLoc, current); err != nil {
		return ConfigPaths{}, fmt.Errorf("write version file: %w", err)
	}

	logConfigSummary(logger, paths, installed, current, stats, hadInstalled)
	return paths, nil
}

func resolveConfigPaths(configDir string) (ConfigPaths, error) {
	cleaned := filepath.Clean(configDir)
	if strings.TrimSpace(cleaned) == "" {
		return ConfigPaths{}, fmt.Errorf("config dir cannot be empty")
	}
	root := filepath.Dir(cleaned)
	subDir := filepath.Base(cleaned)
	return ConfigPaths{
		Root:       root,
		SubDir:     filepath.ToSlash(subDir),
		ConfigDir:  cleaned,
		VersionLoc: filepath.Join(root, "version.json"),
	}, nil
}

func BuildConfigFS(configRoot string) fs.FS {
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

func logConfigSummary(logger *logging.Logger, paths ConfigPaths, installed, current version.VersionInfo, stats config.ExtractStats, hadInstalled bool) {
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
