package plan

import "path/filepath"

func DefaultPath() string {
	return filepath.Join(".gestalt", "PLAN.org")
}
