//go:build !noscip

package scip

import (
	"os"
	"path/filepath"
	"strings"
)

// IsSupportedSourcePath reports whether a path is a supported source file.
func IsSupportedSourcePath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return false
	}

	segments := strings.Split(cleaned, string(os.PathSeparator))
	for _, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		if shouldSkipHashDir(segment) {
			return false
		}
	}

	ext := strings.ToLower(filepath.Ext(cleaned))
	if ext == "" {
		return false
	}
	extensions := extensionsForLanguages(nil)
	_, ok := extensions[ext]
	return ok
}
