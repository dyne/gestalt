package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gestalt/internal/logging"
)

const ioBufferSize = 32 * 1024

type Extractor struct {
	Logger      *logging.Logger
	BackupLimit int
	LastUpdated time.Time
}

type ExtractStats struct {
	Extracted int
	Skipped   int
	BackedUp  int
}

func (e *Extractor) Extract(sourceFS fs.FS, destDir string, manifest map[string]string) error {
	_, err := e.ExtractWithStats(sourceFS, destDir, manifest)
	return err
}

func (e *Extractor) ExtractWithStats(sourceFS fs.FS, destDir string, manifest map[string]string) (ExtractStats, error) {
	stats := ExtractStats{}
	if len(manifest) == 0 {
		return stats, nil
	}
	paths := make([]string, 0, len(manifest))
	for relPath := range manifest {
		paths = append(paths, relPath)
	}
	sort.Strings(paths)

	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(paths) {
		workerCount = len(paths)
	}
	if workerCount <= 1 {
		for _, relPath := range paths {
			fileStats, err := e.extractRel(sourceFS, destDir, relPath, manifest[relPath])
			if err != nil {
				return stats, err
			}
			stats.Extracted += fileStats.Extracted
			stats.Skipped += fileStats.Skipped
			stats.BackedUp += fileStats.BackedUp
		}
		return stats, nil
	}

	jobs := make(chan string)
	var waitGroup sync.WaitGroup
	var statsMu sync.Mutex
	var errOnce sync.Once
	var firstErr error

	worker := func() {
		defer waitGroup.Done()
		for relPath := range jobs {
			fileStats, err := e.extractRel(sourceFS, destDir, relPath, manifest[relPath])
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
				})
				continue
			}
			statsMu.Lock()
			stats.Extracted += fileStats.Extracted
			stats.Skipped += fileStats.Skipped
			stats.BackedUp += fileStats.BackedUp
			statsMu.Unlock()
		}
	}

	waitGroup.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go worker()
	}
	for _, relPath := range paths {
		jobs <- relPath
	}
	close(jobs)
	waitGroup.Wait()

	if firstErr != nil {
		return stats, firstErr
	}
	return stats, nil
}

func (e *Extractor) extractRel(sourceFS fs.FS, destDir, relPath, expectedHash string) (ExtractStats, error) {
	sourcePath := path.Join("config", relPath)
	destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
	return e.extractFile(sourceFS, sourcePath, destPath, expectedHash)
}

func (e *Extractor) extractFile(sourceFS fs.FS, sourcePath, destPath, expectedHash string) (ExtractStats, error) {
	stats := ExtractStats{}
	if info, err := os.Stat(destPath); err == nil {
		if info.IsDir() {
			return stats, fmt.Errorf("destination is a directory: %s", destPath)
		}
		if expectedHash != "" {
			if !e.LastUpdated.IsZero() && !info.ModTime().After(e.LastUpdated) {
				e.logDebug("config file unchanged since last extraction, skipping", map[string]string{
					"path": destPath,
				})
				stats.Skipped++
				return stats, nil
			}
			existingHash, err := hashFile(destPath)
			if err != nil {
				return stats, fmt.Errorf("hash existing file: %w", err)
			}
			if existingHash == expectedHash {
				e.logDebug("config file up-to-date, skipping", map[string]string{
					"path": destPath,
				})
				stats.Skipped++
				return stats, nil
			}
		}
		backupPath, backedUp, err := e.backupExistingFile(destPath)
		if err != nil {
			return stats, err
		}
		if backedUp {
			e.logWarn("config file backed up", map[string]string{
				"path":   destPath,
				"backup": backupPath,
			})
			stats.BackedUp++
		}
	} else if err != nil && !os.IsNotExist(err) {
		return stats, fmt.Errorf("stat destination: %w", err)
	}

	sourceInfo, err := fs.Stat(sourceFS, sourcePath)
	if err != nil {
		return stats, fmt.Errorf("stat source file: %w", err)
	}
	if sourceInfo.IsDir() {
		return stats, fmt.Errorf("source path is a directory: %s", sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return stats, fmt.Errorf("create destination directory: %w", err)
	}
	sourceFile, err := sourceFS.Open(sourcePath)
	if err != nil {
		return stats, fmt.Errorf("open source file: %w", err)
	}
	defer sourceFile.Close()

	if err := writeFileAtomic(destPath, sourceInfo.Mode().Perm(), sourceFile); err != nil {
		return stats, fmt.Errorf("write file: %w", err)
	}
	e.logInfo("config file extracted", map[string]string{
		"path": destPath,
	})
	stats.Extracted++
	return stats, nil
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

	buffer := make([]byte, ioBufferSize)
	if _, err := io.CopyBuffer(tempFile, reader, buffer); err != nil {
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
	buffer := make([]byte, ioBufferSize)
	if _, err := io.CopyBuffer(hasher, file, buffer); err != nil {
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

func (e *Extractor) backupExistingFile(destPath string) (string, bool, error) {
	limit := e.BackupLimit
	if limit <= 0 {
		if err := os.Remove(destPath); err != nil {
			return "", false, fmt.Errorf("remove file without backup: %w", err)
		}
		e.logDebug("config backup disabled, overwriting", map[string]string{
			"path": destPath,
		})
		return "", false, nil
	}

	backupPath := destPath + ".bck"
	if limit > 1 {
		timestamp := time.Now().UTC().Format("20060102-150405-000000000")
		backupPath = destPath + ".bck." + timestamp
	}
	if limit == 1 {
		if err := removeFileIfExists(backupPath); err != nil {
			return "", false, fmt.Errorf("remove backup file: %w", err)
		}
	}
	if err := os.Rename(destPath, backupPath); err != nil {
		return "", false, fmt.Errorf("backup file: %w", err)
	}

	removed, err := cleanupBackups(destPath, limit)
	if err != nil {
		return "", true, err
	}
	for _, entry := range removed {
		e.logDebug("removed old config backup", map[string]string{
			"path":   destPath,
			"backup": entry,
		})
	}
	return backupPath, true, nil
}

type backupEntry struct {
	Path    string
	ModTime time.Time
}

func cleanupBackups(destPath string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	entries, err := listBackupEntries(destPath)
	if err != nil {
		return nil, err
	}
	if len(entries) <= limit {
		return nil, nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ModTime.After(entries[j].ModTime)
	})
	var removed []string
	for _, entry := range entries[limit:] {
		if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, entry.Path)
	}
	return removed, nil
}

func listBackupEntries(destPath string) ([]backupEntry, error) {
	dir := filepath.Dir(destPath)
	base := filepath.Base(destPath)
	prefix := base + ".bck"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var backups []backupEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		backups = append(backups, backupEntry{
			Path:    filepath.Join(dir, entry.Name()),
			ModTime: info.ModTime(),
		})
	}
	return backups, nil
}
