package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gestalt/internal/logging"
)

var promptExtensions = map[string]struct{}{
	".md":   {},
	".tmpl": {},
	".txt":  {},
}

func validatePromptFiles(configDir string, logger *logging.Logger) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		return
	}
	promptsDir := filepath.Join(configDir, "prompts")
	if _, err := os.Stat(promptsDir); err != nil {
		if !os.IsNotExist(err) {
			logPromptWarning(logger, "prompt validation failed", promptsDir, err)
		}
		return
	}

	_ = filepath.WalkDir(promptsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			logPromptWarning(logger, "prompt validation failed", path, err)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !isPromptFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			logPromptWarning(logger, "prompt read failed", path, err)
			return nil
		}
		if !isTextData(data) {
			if logger != nil {
				logger.Warn("prompt file is not valid text", map[string]string{
					"path": path,
				})
			}
		}
		return nil
	})
}

func isPromptFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := promptExtensions[ext]
	return ok
}

func isTextData(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) != -1 {
		return false
	}
	return utf8.Valid(data)
}

func logPromptWarning(logger *logging.Logger, message, path string, err error) {
	if logger == nil {
		return
	}
	fields := map[string]string{
		"path": path,
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	logger.Warn(message, fields)
}
