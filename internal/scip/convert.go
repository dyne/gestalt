package scip

import (
	"fmt"
	"os/exec"
)

// ConvertToSQLite runs the scip CLI to convert a .scip file into a SQLite database.
func ConvertToSQLite(scipPath, dbPath string) error {
	if scipPath == "" {
		return fmt.Errorf("scip path is required")
	}
	if dbPath == "" {
		return fmt.Errorf("database path is required")
	}

	cmd := exec.Command("scip", "expt-convert", "--output", dbPath, scipPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scip expt-convert failed: %w: %s", err, string(output))
	}
	return nil
}
