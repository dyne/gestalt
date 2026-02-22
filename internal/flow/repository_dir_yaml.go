package flow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
)

type DirectoryRepository struct {
	dir    string
	logger *logging.Logger
}

func NewDirectoryRepository(dir string, logger *logging.Logger) *DirectoryRepository {
	return &DirectoryRepository{
		dir:    strings.TrimSpace(dir),
		logger: logger,
	}
}

func (repo *DirectoryRepository) Path() string {
	if repo == nil {
		return ""
	}
	return strings.TrimSpace(repo.dir)
}

func (repo *DirectoryRepository) Load() (Config, error) {
	cfg := DefaultConfig()
	if repo == nil {
		return cfg, errors.New("flow repository unavailable")
	}
	dir := strings.TrimSpace(repo.dir)
	if dir == "" {
		return cfg, errors.New("flow repository path required")
	}

	managedFiles, err := listManagedFlowFiles(dir)
	if err != nil {
		return cfg, err
	}
	defs := ActivityCatalog()
	for _, name := range managedFiles {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return DefaultConfig(), err
		}
		flowFile, err := decodeFlowFileYAML(data)
		if err != nil {
			repo.logWarn("flow file invalid", map[string]string{
				"path":  path,
				"error": err.Error(),
			})
			return DefaultConfig(), ErrInvalidConfig
		}
		addFlowFile(&cfg, flowFile)
	}
	if err := ValidateConfig(cfg, defs); err != nil {
		repo.logWarn("flow config invalid", map[string]string{
			"error": err.Error(),
		})
		return DefaultConfig(), ErrInvalidConfig
	}
	return cfg, nil
}

func (repo *DirectoryRepository) Save(cfg Config) error {
	if repo == nil {
		return errors.New("flow repository unavailable")
	}
	dir := strings.TrimSpace(repo.dir)
	if dir == "" {
		return errors.New("flow repository path required")
	}
	if err := validateManagedFilenameCollisions(cfg.Triggers); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	desired := map[string]struct{}{}
	for _, trigger := range cfg.Triggers {
		name, err := managedFlowFilename(trigger.ID)
		if err != nil {
			return err
		}
		desired[name] = struct{}{}
		flowFile := FlowFile{
			ID:        trigger.ID,
			Label:     trigger.Label,
			EventType: trigger.EventType,
			Where:     FlowWhere(cloneStringMap(trigger.Where)),
			Bindings:  cloneFlowBindings(cfg.BindingsByTriggerID[trigger.ID]),
		}
		payload, err := encodeFlowFileYAML(flowFile)
		if err != nil {
			return err
		}
		if err := writeManagedFileAtomic(dir, name, payload); err != nil {
			return err
		}
	}

	if err := removeStaleManagedFlowFiles(dir, desired); err != nil {
		return err
	}
	return nil
}

func addFlowFile(cfg *Config, file FlowFile) {
	if cfg == nil {
		return
	}
	cfg.Triggers = append(cfg.Triggers, EventTrigger{
		ID:        file.ID,
		Label:     file.Label,
		EventType: file.EventType,
		Where:     cloneStringMap(file.Where),
	})
	cfg.BindingsByTriggerID[file.ID] = cloneFlowBindingsToDomain(file.Bindings)
}

func cloneStringMap(source map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneFlowBindings(source []ActivityBinding) []FlowBinding {
	out := make([]FlowBinding, 0, len(source))
	for _, binding := range source {
		out = append(out, FlowBinding{
			ActivityID: binding.ActivityID,
			Config:     FlowBindingConfig(cloneAnyMap(binding.Config)),
		})
	}
	return out
}

func cloneFlowBindingsToDomain(source []FlowBinding) []ActivityBinding {
	out := make([]ActivityBinding, 0, len(source))
	for _, binding := range source {
		out = append(out, ActivityBinding{
			ActivityID: binding.ActivityID,
			Config:     cloneAnyMap(map[string]any(binding.Config)),
		})
	}
	return out
}

func cloneAnyMap(source map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func writeManagedFileAtomic(dir string, name string, payload []byte) error {
	tempFile, err := os.CreateTemp(dir, name+".tmp-*")
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
	if err := os.Rename(tempName, filepath.Join(dir, name)); err != nil {
		return err
	}
	return nil
}

func (repo *DirectoryRepository) logWarn(message string, fields map[string]string) {
	if repo == nil || repo.logger == nil {
		return
	}
	repo.logger.Warn(message, fields)
}
