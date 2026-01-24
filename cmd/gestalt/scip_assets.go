//go:build !noscip

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gestalt"
	"gestalt/internal/logging"
	"gestalt/internal/scip"
)

func prepareScipAssets(logger *logging.Logger) error {
	destDir := filepath.Join(".gestalt", "scip")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create scip assets dir: %w", err)
	}

	manifest, err := scip.LoadAssetsManifest(gestalt.EmbeddedScipAssetsFS)
	if err != nil {
		if errors.Is(err, scip.ErrAssetsManifestMissing) {
			if logger != nil {
				logger.Warn("scip assets manifest missing, computing hashes at startup", nil)
			}
			manifest, err = scip.BuildAssetsManifest(gestalt.EmbeddedScipAssetsFS)
		}
		if err != nil {
			return fmt.Errorf("load scip assets manifest: %w", err)
		}
	}

	if len(manifest) == 0 {
		return nil
	}

	stats, err := scip.ExtractAssets(gestalt.EmbeddedScipAssetsFS, destDir, manifest)
	if err != nil {
		return fmt.Errorf("extract scip assets: %w", err)
	}
	if logger != nil {
		logger.Info("scip assets extracted", map[string]string{
			"extracted": strconv.Itoa(stats.Extracted),
			"skipped":   strconv.Itoa(stats.Skipped),
		})
	}
	return nil
}
