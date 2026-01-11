package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gestalt/internal/version"
)

var ErrVersionFileMissing = errors.New("version file not found")

func LoadVersionFile(path string) (version.VersionInfo, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return version.VersionInfo{}, ErrVersionFileMissing
		}
		return version.VersionInfo{}, err
	}
	var info version.VersionInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return version.VersionInfo{}, err
	}
	return info, nil
}

func WriteVersionFile(path string, info version.VersionInfo) error {
	payload, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}
