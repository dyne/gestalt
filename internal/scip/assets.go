//go:build !noscip

package scip

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const scipAssetsRoot = "assets/scip"
const scipAssetsManifestPath = "assets/scip/manifest.json"
const scipAssetsBufferSize = 32 * 1024

var ErrAssetsManifestMissing = errors.New("scip assets manifest not found")

type AssetExtractStats struct {
	Extracted int
	Skipped   int
}

func LoadAssetsManifest(fsys fs.FS) (map[string]string, error) {
	payload, err := fs.ReadFile(fsys, scipAssetsManifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrAssetsManifestMissing
		}
		return nil, err
	}
	manifest := make(map[string]string)
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func BuildAssetsManifest(fsys fs.FS) (map[string]string, error) {
	manifest := make(map[string]string)
	err := fs.WalkDir(fsys, scipAssetsRoot, func(entryPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entryPath == scipAssetsManifestPath {
			return nil
		}
		relative := strings.TrimPrefix(entryPath, scipAssetsRoot+"/")
		hash, err := hashAssetFSFile(fsys, entryPath)
		if err != nil {
			return err
		}
		manifest[relative] = hash
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return manifest, nil
		}
		return nil, err
	}
	return manifest, nil
}

func ExtractAssets(fsys fs.FS, destDir string, manifest map[string]string) (AssetExtractStats, error) {
	stats := AssetExtractStats{}
	if len(manifest) == 0 {
		return stats, nil
	}
	paths := make([]string, 0, len(manifest))
	for relPath := range manifest {
		paths = append(paths, relPath)
	}
	sort.Strings(paths)

	for _, relPath := range paths {
		expectedHash := manifest[relPath]
		sourcePath := path.Join(scipAssetsRoot, relPath)
		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))

		if info, err := os.Stat(destPath); err == nil {
			if info.IsDir() {
				return stats, fmt.Errorf("destination is a directory: %s", destPath)
			}
			if expectedHash != "" {
				currentHash, err := hashAssetFile(destPath)
				if err != nil {
					return stats, err
				}
				if currentHash == expectedHash {
					stats.Skipped++
					continue
				}
			}
		} else if err != nil && !os.IsNotExist(err) {
			return stats, err
		}

		sourceInfo, err := fs.Stat(fsys, sourcePath)
		if err != nil {
			return stats, err
		}
		if sourceInfo.IsDir() {
			return stats, fmt.Errorf("source path is a directory: %s", sourcePath)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return stats, err
		}
		sourceFile, err := fsys.Open(sourcePath)
		if err != nil {
			return stats, err
		}
		if err := writeFileAtomic(destPath, sourceInfo.Mode().Perm(), sourceFile); err != nil {
			sourceFile.Close()
			return stats, err
		}
		sourceFile.Close()
		stats.Extracted++
	}
	return stats, nil
}

func hashAssetFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return hashAssetReader(file)
}

func hashAssetFSFile(fsys fs.FS, entryPath string) (string, error) {
	file, err := fsys.Open(entryPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return hashAssetReader(file)
}

func hashAssetReader(reader io.Reader) (string, error) {
	hasher := fnv.New64a()
	buffer := make([]byte, scipAssetsBufferSize)
	if _, err := io.CopyBuffer(hasher, reader, buffer); err != nil {
		return "", err
	}
	return fmt.Sprintf("%016x", hasher.Sum64()), nil
}

func writeFileAtomic(destPath string, mode fs.FileMode, reader io.Reader) error {
	dir := filepath.Dir(destPath)
	tempFile, err := os.CreateTemp(dir, ".gestalt-scip-*")
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	buffer := make([]byte, scipAssetsBufferSize)
	if _, err := io.CopyBuffer(tempFile, reader, buffer); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempFile.Name(), destPath)
}
