package scip

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IndexMetadata records index generation context for freshness checks.
type IndexMetadata struct {
	CreatedAt   time.Time `json:"created_at"`
	ProjectRoot string    `json:"project_root"`
	Languages   []string  `json:"languages,omitempty"`
	FilesHashed string    `json:"files_hashed"`
}

// MetadataPath returns the sidecar path for index metadata.
func MetadataPath(indexPath string) string {
	return indexPath + ".meta.json"
}

// LoadMetadata reads index metadata from disk.
func LoadMetadata(indexPath string) (IndexMetadata, error) {
	path := MetadataPath(indexPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return IndexMetadata{}, err
	}

	var metadata IndexMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return IndexMetadata{}, err
	}
	return metadata, nil
}

// SaveMetadata writes index metadata to disk.
func SaveMetadata(indexPath string, metadata IndexMetadata) error {
	path := MetadataPath(indexPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	payload, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// BuildMetadata creates metadata for a project root.
func BuildMetadata(projectRoot string, languages []string) (IndexMetadata, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return IndexMetadata{}, fmt.Errorf("project root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	normalized := normalizeLanguages(languages)
	hash, err := HashSourceFiles(absRoot, normalized)
	if err != nil {
		return IndexMetadata{}, err
	}

	return IndexMetadata{
		CreatedAt:   time.Now().UTC(),
		ProjectRoot: absRoot,
		Languages:   normalized,
		FilesHashed: hash,
	}, nil
}

// IsFresh reports whether the project files match the stored metadata hash.
func IsFresh(metadata IndexMetadata) (bool, error) {
	if strings.TrimSpace(metadata.ProjectRoot) == "" {
		return false, fmt.Errorf("metadata project root is required")
	}
	current, err := HashSourceFiles(metadata.ProjectRoot, metadata.Languages)
	if err != nil {
		return false, err
	}
	return current == metadata.FilesHashed, nil
}

// HashSourceFiles computes a hash for source files under a root directory.
func HashSourceFiles(root string, languages []string) (string, error) {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return "", fmt.Errorf("root is required")
	}

	extensions := extensionsForLanguages(languages)
	var files []string

	err := filepath.WalkDir(trimmedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipHashDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, ok := extensions[ext]; !ok {
			return nil
		}
		rel, err := filepath.Rel(trimmedRoot, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	hasher := sha256.New()
	for _, rel := range files {
		fullPath := filepath.Join(trimmedRoot, filepath.FromSlash(rel))
		if err := hashFile(hasher, rel, fullPath); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func extensionsForLanguages(languages []string) map[string]struct{} {
	extensions := make(map[string]struct{})
	if len(languages) == 0 {
		for _, set := range languageExtensions {
			for _, ext := range set {
				extensions[ext] = struct{}{}
			}
		}
		return extensions
	}
	for _, language := range languages {
		if set, ok := languageExtensions[language]; ok {
			for _, ext := range set {
				extensions[ext] = struct{}{}
			}
		}
	}
	return extensions
}

var languageExtensions = map[string][]string{
	"go":         {".go"},
	"typescript": {".ts", ".tsx", ".js", ".jsx"},
	"python":     {".py"},
	"java":       {".java"},
}

func normalizeLanguages(languages []string) []string {
	if len(languages) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(languages))
	var normalized []string
	for _, language := range languages {
		trimmed := strings.ToLower(strings.TrimSpace(language))
		if trimmed == "" {
			continue
		}
		if _, ok := languageExtensions[trimmed]; !ok {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

func shouldSkipHashDir(name string) bool {
	switch name {
	case ".git", ".gestalt", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func hashFile(hasher io.Writer, relativePath, fullPath string) error {
	if _, err := io.WriteString(hasher, relativePath); err != nil {
		return err
	}
	if _, err := hasher.Write([]byte{0}); err != nil {
		return err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(hasher, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	if _, err := hasher.Write([]byte{0}); err != nil {
		return err
	}
	return nil
}
