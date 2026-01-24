package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"gestalt/internal/fsutil"
	"gestalt/internal/logging"
)

// Loader reads skill packages from the filesystem.
type Loader struct {
	Logger *logging.Logger
}

// Load scans a directory for skill packages and returns a map keyed by skill ID.
func (l Loader) Load(skillFS fs.FS, dir string) (map[string]*Skill, error) {
	skillFS, dir, err := normalizeSkillPath(skillFS, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]*Skill{}, nil
		}
		return nil, err
	}

	entries, err := fsutil.ReadDirOrEmpty(skillFS, dir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	skills := make(map[string]*Skill)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillID := entry.Name()
		skillDir := path.Join(dir, skillID)
		subFS, err := fs.Sub(skillFS, skillDir)
		if err != nil {
			l.warnLoadError(skillID, skillDir, err)
			continue
		}
		skillPath := path.Join(skillDir, "SKILL.md")
		data, err := fs.ReadFile(subFS, "SKILL.md")
		if err != nil {
			l.warnLoadError(skillID, skillPath, err)
			continue
		}
		skill, err := Parse(data)
		if err != nil {
			l.warnLoadError(skillID, skillPath, err)
			continue
		}
		skill.Path = skillDir
		if err := skill.ValidateFS(skillFS); err != nil {
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

func normalizeSkillPath(skillFS fs.FS, dir string) (fs.FS, string, error) {
	fsys, cleaned, err := fsutil.NormalizeFSPaths(skillFS, "skill loader", dir)
	if err != nil {
		return nil, "", err
	}
	if len(cleaned) == 0 {
		return fsys, ".", nil
	}
	return fsys, cleaned[0], nil
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
