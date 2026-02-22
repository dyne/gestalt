package flow

import (
	"errors"
	"path/filepath"
	"strings"

	"gestalt/internal/logging"
)

var ErrInvalidConfig = errors.New("flow config invalid")

type Repository interface {
	Load() (Config, error)
	Save(Config) error
}

// NewFileRepository is kept for compatibility and now returns a YAML directory repository.
func NewFileRepository(path string, logger *logging.Logger) Repository {
	trimmed := strings.TrimSpace(path)
	if filepath.Ext(trimmed) != "" {
		trimmed = filepath.Dir(trimmed)
	}
	return NewDirectoryRepository(trimmed, logger)
}
