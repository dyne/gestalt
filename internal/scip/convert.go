//go:build !noscip

package scip

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ConvertToSQLite runs the scip CLI to convert a .scip file into a SQLite database.
func ConvertToSQLite(scipPath, dbPath string) error {
	if scipPath == "" {
		return fmt.Errorf("scip path is required")
	}
	if dbPath == "" {
		return fmt.Errorf("database path is required")
	}

	outputPath := dbPath
	tempPath := ""
	if fileExists(dbPath) {
		tempPath = dbPath + ".tmp"
		_ = os.Remove(tempPath)
		outputPath = tempPath
	}

	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create sqlite directory: %w", err)
		}
	}

	cmd := exec.Command("scip", "expt-convert", "--output", outputPath, scipPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scip expt-convert failed: %w: %s", err, string(output))
	}
	if tempPath != "" {
		if err := replaceSQLiteDB(tempPath, dbPath); err != nil {
			return err
		}
	}
	return nil
}

func replaceSQLiteDB(tempPath, dbPath string) error {
	_ = cleanupSQLiteSidecars(dbPath)
	if err := os.Rename(tempPath, dbPath); err == nil {
		return nil
	}

	if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove locked sqlite database: %w", err)
	}
	_ = cleanupSQLiteSidecars(dbPath)
	if err := os.Rename(tempPath, dbPath); err != nil {
		return fmt.Errorf("replace sqlite database: %w", err)
	}
	return nil
}

func cleanupSQLiteSidecars(dbPath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove sqlite %s: %w", suffix, err)
		}
	}
	return nil
}
