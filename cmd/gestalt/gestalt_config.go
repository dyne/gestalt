package main

import (
	"os"
	"path/filepath"
)

const gestaltConfigFilename = "gestalt.toml"

func gestaltConfigPath(cfg Config, paths configPaths) string {
	if cfg.DevMode {
		return filepath.Join("config", gestaltConfigFilename)
	}
	return filepath.Join(paths.ConfigDir, gestaltConfigFilename)
}

func loadGestaltConfig(cfg Config, paths configPaths) ([]byte, string, error) {
	path := gestaltConfigPath(cfg, paths)
	payload, err := os.ReadFile(path)
	return payload, path, err
}
