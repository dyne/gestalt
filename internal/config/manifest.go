package config

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
)

var ErrManifestMissing = errors.New("config manifest not found")

const manifestPath = "config/manifest.json"

func LoadManifest(configFS embed.FS) (map[string]string, error) {
	return loadManifestFromPath(configFS, manifestPath)
}

func loadManifestFromPath(configFS fs.FS, path string) (map[string]string, error) {
	payload, err := fs.ReadFile(configFS, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrManifestMissing
		}
		return nil, err
	}
	manifest := make(map[string]string)
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}
