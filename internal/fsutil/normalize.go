package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// CleanFSPath normalizes a filesystem path for use with fs.FS.
func CleanFSPath(pathValue string) (string, error) {
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

// NormalizeFSPaths prepares paths for use with fs.FS, returning a shared root FS and cleaned paths.
func NormalizeFSPaths(rootFS fs.FS, label string, paths ...string) (fs.FS, []string, error) {
	if len(paths) == 0 {
		return rootFS, nil, nil
	}
	if rootFS != nil {
		cleaned := make([]string, len(paths))
		for i, value := range paths {
			normalized, err := CleanFSPath(value)
			if err != nil {
				return nil, nil, err
			}
			cleaned[i] = normalized
		}
		return rootFS, cleaned, nil
	}

	absPaths := make([]string, len(paths))
	for i, value := range paths {
		abs, err := filepath.Abs(value)
		if err != nil {
			abs = value
		}
		absPaths[i] = abs
	}

	volume := filepath.VolumeName(absPaths[0])
	for _, abs := range absPaths[1:] {
		if filepath.VolumeName(abs) != volume {
			prefix := "paths"
			if strings.TrimSpace(label) != "" {
				prefix = label + " paths"
			}
			return nil, nil, fmt.Errorf("%s span volumes: %q, %q", prefix, absPaths[0], abs)
		}
	}

	root := string(os.PathSeparator)
	if volume != "" {
		root = volume + string(os.PathSeparator)
	}

	cleaned := make([]string, len(paths))
	for i, abs := range absPaths {
		rel := strings.TrimPrefix(abs, root)
		normalized, err := CleanFSPath(rel)
		if err != nil {
			return nil, nil, err
		}
		cleaned[i] = normalized
	}

	return os.DirFS(root), cleaned, nil
}

// ReadDirOrEmpty returns an empty slice when the directory does not exist.
func ReadDirOrEmpty(rootFS fs.FS, dir string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(rootFS, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}
