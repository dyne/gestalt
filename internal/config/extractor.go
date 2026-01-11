package config

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"gestalt/internal/logging"
)

type Extractor struct {
	Logger *logging.Logger
}

func (e *Extractor) Extract(sourceFS embed.FS, destDir string, manifest map[string]string) error {
	if len(manifest) == 0 {
		return nil
	}
	paths := make([]string, 0, len(manifest))
	for relPath := range manifest {
		paths = append(paths, relPath)
	}
	sort.Strings(paths)

	for _, relPath := range paths {
		expectedHash := manifest[relPath]
		sourcePath := path.Join("config", relPath)
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		if err := e.extractFile(sourceFS, sourcePath, destPath, expectedHash); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) extractFile(sourceFS fs.FS, sourcePath, destPath, expectedHash string) error {
	if info, err := os.Stat(destPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("destination is a directory: %s", destPath)
		}
		existingHash, err := hashFile(destPath)
		if err != nil {
			return fmt.Errorf("hash existing file: %w", err)
		}
		if existingHash == expectedHash {
			e.logDebug("config file up-to-date, skipping", map[string]string{
				"path": destPath,
			})
			return nil
		}
		backupPath := destPath + ".bck"
		if err := removeFileIfExists(backupPath); err != nil {
			return fmt.Errorf("remove backup file: %w", err)
		}
		if err := os.Rename(destPath, backupPath); err != nil {
			return fmt.Errorf("backup file: %w", err)
		}
		e.logWarn("config file backed up", map[string]string{
			"path":   destPath,
			"backup": backupPath,
		})
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat destination: %w", err)
	}

	sourceInfo, err := fs.Stat(sourceFS, sourcePath)
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source path is a directory: %s", sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	sourceFile, err := sourceFS.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer sourceFile.Close()

	if err := writeFileAtomic(destPath, sourceInfo.Mode().Perm(), sourceFile); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	e.logInfo("config file extracted", map[string]string{
		"path": destPath,
	})
	return nil
}

func writeFileAtomic(destPath string, mode fs.FileMode, reader io.Reader) error {
	dir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(dir, ".gestalt-config-*")
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	if _, err := io.Copy(tempFile, reader); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempFile.Name(), destPath); err != nil {
		return err
	}
	if err := os.Chmod(destPath, mode); err != nil {
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func removeFileIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (e *Extractor) logDebug(message string, fields map[string]string) {
	if e == nil || e.Logger == nil {
		return
	}
	e.Logger.Debug(message, fields)
}

func (e *Extractor) logInfo(message string, fields map[string]string) {
	if e == nil || e.Logger == nil {
		return
	}
	e.Logger.Info(message, fields)
}

func (e *Extractor) logWarn(message string, fields map[string]string) {
	if e == nil || e.Logger == nil {
		return
	}
	e.Logger.Warn(message, fields)
}
