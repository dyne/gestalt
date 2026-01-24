//go:build noscip

package main

import "gestalt/internal/logging"

func prepareScipAssets(logger *logging.Logger) error {
	if logger != nil {
		logger.Info("scip assets disabled at build time", nil)
	}
	return nil
}
