package prompt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ResolvePromptPath returns the first matching prompt file path for promptName.
func ResolvePromptPath(promptFS fs.FS, promptDir, promptName string) (string, error) {
	promptName = strings.TrimSpace(promptName)
	if promptName == "" {
		return "", errors.New("prompt name is required")
	}
	candidates := promptCandidates(promptName)
	if promptFS != nil {
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			promptPath := path.Join(promptDir, candidate)
			if _, err := fs.Stat(promptFS, promptPath); err == nil {
				return promptPath, nil
			} else if !errors.Is(err, fs.ErrNotExist) {
				return "", err
			}
		}
		return "", fmt.Errorf("prompt %q not found", promptName)
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		promptPath := filepath.Join(promptDir, candidate)
		if _, err := os.Stat(promptPath); err == nil {
			return promptPath, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("prompt %q not found", promptName)
}
