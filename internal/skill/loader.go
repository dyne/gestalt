package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"gestalt/internal/logging"
)

// Loader reads skill packages from the filesystem.
type Loader struct {
	Logger *logging.Logger
}

// Load scans a directory for skill packages and returns a map keyed by skill ID.
func (l Loader) Load(dir string) (map[string]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Skill{}, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	skills := make(map[string]*Skill)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := entry.Name()
		skillPath := filepath.Join(dir, skillID, "SKILL.md")
		skill, err := ParseFile(skillPath)
		if err != nil {
			l.warnLoadError(skillID, skillPath, err)
			continue
		}
		if _, exists := skills[skillID]; exists {
			l.warnDuplicate(skillID, skillPath)
			continue
		}
		skills[skillID] = skill
	}

	return skills, nil
}

func (l Loader) warnLoadError(skillID, path string, err error) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn("skill load failed", map[string]string{
		"skill_id": skillID,
		"path":     path,
		"error":    err.Error(),
	})
}

func (l Loader) warnDuplicate(skillID, path string) {
	if l.Logger == nil {
		return
	}
	l.Logger.Warn("skill duplicate ignored", map[string]string{
		"skill_id": skillID,
		"path":     path,
	})
}
