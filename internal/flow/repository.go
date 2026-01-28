package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gestalt/internal/logging"
)

var ErrInvalidConfig = errors.New("flow config invalid")

type Repository interface {
	Load() (Config, error)
	Save(Config) error
}

type FileRepository struct {
	path   string
	logger *logging.Logger
}

func NewFileRepository(path string, logger *logging.Logger) *FileRepository {
	return &FileRepository{path: path, logger: logger}
}

func (repo *FileRepository) Load() (Config, error) {
	cfg := DefaultConfig()
	if repo == nil {
		return cfg, errors.New("flow repository unavailable")
	}
	trimmedPath := strings.TrimSpace(repo.path)
	if trimmedPath == "" {
		return cfg, errors.New("flow repository path required")
	}
	data, err := os.ReadFile(trimmedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		repo.backupCorruptFile(trimmedPath, err)
		return DefaultConfig(), ErrInvalidConfig
	}
	if cfg.Version != ConfigVersion {
		repo.backupCorruptFile(trimmedPath, fmt.Errorf("unsupported version %d", cfg.Version))
		return DefaultConfig(), ErrInvalidConfig
	}
	if cfg.Triggers == nil {
		cfg.Triggers = []EventTrigger{}
	}
	if cfg.BindingsByTriggerID == nil {
		cfg.BindingsByTriggerID = map[string][]ActivityBinding{}
	}
	return cfg, nil
}

func (repo *FileRepository) Save(cfg Config) error {
	if repo == nil {
		return errors.New("flow repository unavailable")
	}
	trimmedPath := strings.TrimSpace(repo.path)
	if trimmedPath == "" {
		return errors.New("flow repository path required")
	}
	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}
	if cfg.Triggers == nil {
		cfg.Triggers = []EventTrigger{}
	}
	if cfg.BindingsByTriggerID == nil {
		cfg.BindingsByTriggerID = map[string][]ActivityBinding{}
	}

	dir := filepath.Dir(trimmedPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, "automations-*.json")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}()

	if _, err := tempFile.Write(payload); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tempName, trimmedPath); err != nil {
		return err
	}
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

func (repo *FileRepository) backupCorruptFile(path string, cause error) {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	backupPath := fmt.Sprintf("%s.%s.bck", path, timestamp)
	if err := os.Rename(path, backupPath); err != nil {
		repo.logWarn("flow config backup failed", map[string]string{
			"path":  path,
			"error": err.Error(),
		})
		return
	}
	repo.logWarn("flow config backed up", map[string]string{
		"path":   path,
		"backup": backupPath,
		"error":  cause.Error(),
	})
}

func (repo *FileRepository) logWarn(message string, fields map[string]string) {
	if repo == nil || repo.logger == nil {
		return
	}
	repo.logger.Warn(message, fields)
}
