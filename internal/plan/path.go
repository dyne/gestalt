package plan

import "path/filepath"

func DefaultPlansDir() string {
	return filepath.Join(".gestalt", "plans")
}
