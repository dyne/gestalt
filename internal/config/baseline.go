package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var ErrBaselineManifestMissing = errors.New("baseline manifest not found")

const baselineManifestName = ".gestalt-baseline-manifest.json"

func LoadBaselineManifest(destDir string) (map[string]string, error) {
	manifestPath := filepath.Join(destDir, baselineManifestName)
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBaselineManifestMissing
		}
		return nil, err
	}
	manifest := make(map[string]string)
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func WriteBaselineManifest(destDir string, manifest map[string]string) error {
	if manifest == nil {
		manifest = map[string]string{}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	manifestPath := filepath.Join(destDir, baselineManifestName)
	return writeFileAtomic(manifestPath, 0o644, bytes.NewReader(payload))
}
