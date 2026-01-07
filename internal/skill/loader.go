package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	entries, err := fs.ReadDir(skillFS, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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
	if skillFS != nil {
		cleanDir, err := cleanFSPath(dir)
		if err != nil {
			return nil, "", err
		}
		return skillFS, cleanDir, nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	volume := filepath.VolumeName(absDir)
	root := string(os.PathSeparator)
	if volume != "" {
		root = volume + string(os.PathSeparator)
	}

	relDir := strings.TrimPrefix(absDir, root)
	cleanDir, err := cleanFSPath(relDir)
	if err != nil {
		return nil, "", err
	}
	return os.DirFS(root), cleanDir, nil
}

func cleanFSPath(pathValue string) (string, error) {
	slashPath := filepath.ToSlash(pathValue)
	slashPath = strings.TrimPrefix(slashPath, "/")
	if slashPath == "" {
		return ".", nil
	}
	cleaned := path.Clean(slashPath)
	if cleaned == "." {
		return ".", nil
	}
	if !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("invalid fs path: %q", pathValue)
	}
	return cleaned, nil
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
