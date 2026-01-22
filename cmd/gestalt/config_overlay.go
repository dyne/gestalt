package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type overlayFS struct {
	primary  fs.FS
	fallback fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	file, err := o.primary.Open(name)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return o.fallback.Open(name)
	}
	return nil, err
}

func shouldPreferLocalConfig(paths configPaths) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	defaultConfigDir := filepath.Join(cwd, ".gestalt", "config")
	absConfigDir, err := filepath.Abs(paths.ConfigDir)
	if err != nil {
		absConfigDir = paths.ConfigDir
	}
	return filepath.Clean(absConfigDir) == filepath.Clean(defaultConfigDir)
}
