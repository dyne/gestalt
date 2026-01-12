package scip

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Indexer represents a SCIP indexer for a language.
type Indexer struct {
	Language    string
	Name        string
	Version     string
	Binary      string
	URL         string
	URLTemplate string
}

var builtInIndexers = []Indexer{
	{
		Language: "go",
		Name:     "scip-go",
		Version:  "v0.1.26",
		Binary:   "scip-go",
	},
	{
		Language: "typescript",
		Name:     "scip-typescript",
		Version:  "v0.3.13",
		Binary:   "scip-typescript",
	},
	{
		Language: "python",
		Name:     "scip-python",
		Version:  "v0.3.9",
		Binary:   "scip-python",
	},
}

var indexerDirOverride string

// DownloadIndexer downloads the indexer binary to ~/.gestalt/indexers/.
func DownloadIndexer(lang string) error {
	indexer, err := findIndexer(lang)
	if err != nil {
		return err
	}

	indexerDir, err := getIndexerDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(indexerDir, 0o755); err != nil {
		return fmt.Errorf("create indexer dir: %w", err)
	}

	indexerPath, err := indexerBinaryPath(indexer)
	if err != nil {
		return err
	}
	url, err := indexerURL(indexer)
	if err != nil {
		return err
	}
	if err := validateAssetVersion(indexer, url); err != nil {
		return err
	}
	if err := downloadBinary(url, indexerPath); err != nil {
		return err
	}
	return nil
}

// DetectLanguages scans a directory for language markers.
func DetectLanguages(dir string) ([]string, error) {
	languages := []string{}
	seen := map[string]struct{}{}

	if hasMarkerFile(dir, "go.mod") || hasFileExtension(dir, []string{".go"}) {
		addLanguage(&languages, seen, "go")
	}
	if hasMarkerFile(dir, "package.json") || hasMarkerFile(dir, "tsconfig.json") ||
		hasFileExtension(dir, []string{".ts", ".tsx", ".js", ".jsx"}) {
		addLanguage(&languages, seen, "typescript")
	}
	if hasMarkerFile(dir, "setup.py") || hasMarkerFile(dir, "requirements.txt") ||
		hasFileExtension(dir, []string{".py"}) {
		addLanguage(&languages, seen, "python")
	}
	if hasMarkerFile(dir, "pom.xml") || hasMarkerFile(dir, "build.gradle") ||
		hasFileExtension(dir, []string{".java"}) {
		addLanguage(&languages, seen, "java")
	}

	return languages, nil
}

// RunIndexer executes an indexer on a directory and writes index.scip output.
func RunIndexer(lang, dir, output string) error {
	if output == "" {
		output = "index.scip"
	}
	indexer, err := findIndexer(lang)
	if err != nil {
		return err
	}
	indexerPath, err := indexerBinaryPath(indexer)
	if err != nil {
		return err
	}
	if !fileExists(indexerPath) {
		if err := DownloadIndexer(lang); err != nil {
			return err
		}
	}

	args, err := indexerArgs(indexer, output)
	if err != nil {
		return err
	}

	cmd := exec.Command(indexerPath, args...)
	cmd.Dir = dir
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("indexer %s failed: %w: %s", indexer.Name, err, string(outputBytes))
	}
	return nil
}

func EnsureIndexer(lang string) (string, error) {
	indexer, err := findIndexer(lang)
	if err != nil {
		return "", err
	}
	indexerPath, err := indexerBinaryPath(indexer)
	if err != nil {
		return "", err
	}
	if !fileExists(indexerPath) {
		if err := DownloadIndexer(lang); err != nil {
			return "", err
		}
	}
	return indexerPath, nil
}

func findIndexer(lang string) (Indexer, error) {
	for _, indexer := range builtInIndexers {
		if indexer.Language == lang {
			return indexer, nil
		}
	}
	return Indexer{}, fmt.Errorf("unknown indexer language: %s", lang)
}

func getIndexerDir() (string, error) {
	if indexerDirOverride != "" {
		return indexerDirOverride, nil
	}
	path := filepath.Join(".gestalt", "scip")
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return abs, nil
}

func indexerBinaryPath(indexer Indexer) (string, error) {
	indexerDir, err := getIndexerDir()
	if err != nil {
		return "", err
	}
	binary := indexer.Binary
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(binary), ".exe") {
		binary += ".exe"
	}
	return filepath.Join(indexerDir, binary), nil
}

func indexerURL(indexer Indexer) (string, error) {
	if indexer.URL != "" {
		return indexer.URL, nil
	}
	if indexer.URLTemplate != "" {
		asset, err := indexerAssetName(indexer)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(indexer.URLTemplate, indexer.Version, asset), nil
	}
	if indexer.Version == "" {
		return "", fmt.Errorf("indexer %s has no version", indexer.Name)
	}
	asset, err := indexerAssetName(indexer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/sourcegraph/%s/releases/download/%s/%s", indexer.Name, indexer.Version, asset), nil
}

func platformSuffix(indexer Indexer) string {
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
}

func indexerAssetName(indexer Indexer) (string, error) {
	switch indexer.Name {
	case "scip-go":
		version := normalizedVersion(indexer.Version)
		return fmt.Sprintf("scip-go_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH), nil
	case "scip-typescript":
		osName, err := nodeOSName(runtime.GOOS)
		if err != nil {
			return "", err
		}
		archName, err := nodeArchName(runtime.GOARCH)
		if err != nil {
			return "", err
		}
		version := normalizedVersion(indexer.Version)
		return fmt.Sprintf("scip-typescript_%s_%s_%s.tar.gz", version, osName, archName), nil
	case "scip-python":
		osName, err := nodeOSName(runtime.GOOS)
		if err != nil {
			return "", err
		}
		archName, err := nodeArchName(runtime.GOARCH)
		if err != nil {
			return "", err
		}
		version := normalizedVersion(indexer.Version)
		return fmt.Sprintf("scip-python_%s_%s_%s.tar.gz", version, osName, archName), nil
	default:
		return "", fmt.Errorf("unknown indexer asset pattern: %s", indexer.Name)
	}
}

func nodeOSName(goos string) (string, error) {
	switch goos {
	case "linux":
		return "linux", nil
	case "darwin":
		return "macos", nil
	case "windows":
		return "windows", nil
	default:
		return "", fmt.Errorf("unsupported os: %s", goos)
	}
}

func nodeArchName(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch: %s", goarch)
	}
}

func indexerArgs(indexer Indexer, output string) ([]string, error) {
	switch indexer.Language {
	case "go":
		return []string{
			"--output", output,
			"--project-root", ".",
			"--module-root", ".",
			"--repository-root", ".",
			"--skip-tests",
		}, nil
	case "typescript", "python":
		return []string{"index", "--output", output}, nil
	default:
		return nil, fmt.Errorf("unsupported language: %s", indexer.Language)
	}
}

func downloadBinary(url, destination string) error {
	if strings.HasPrefix(url, "file://") {
		sourcePath := strings.TrimPrefix(url, "file://")
		source, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("open source indexer: %w", err)
		}
		defer source.Close()
		if isTarGz(url) {
			return extractTarGz(source, destination)
		}
		return copyToDestination(source, destination)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download indexer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download indexer: unexpected status %d", resp.StatusCode)
	}

	if isTarGz(url) {
		return extractTarGz(resp.Body, destination)
	}
	return copyToDestination(resp.Body, destination)
}

func copyToDestination(source io.Reader, destination string) error {
	tempPath := destination + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(file, source); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write indexer: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close indexer: %w", err)
	}

	return finalizeDownloadedBinary(tempPath, destination)
}

func finalizeDownloadedBinary(tempPath, destination string) error {
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tempPath, 0o755); err != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("chmod indexer: %w", err)
		}
	}
	if err := os.Rename(tempPath, destination); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename indexer: %w", err)
	}
	return nil
}

func extractTarGz(source io.Reader, destination string) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("read gzip: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	target := filepath.Base(destination)
	altTarget := strings.TrimSuffix(target, ".exe")

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(header.Name)
		if name != target && name != altTarget {
			continue
		}
		return copyToDestination(tarReader, destination)
	}
	return fmt.Errorf("indexer archive missing %s", target)
}

func isTarGz(url string) bool {
	return strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz")
}

func normalizedVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func validateAssetVersion(indexer Indexer, url string) error {
	version := normalizedVersion(indexer.Version)
	if version == "" {
		return nil
	}
	base := path.Base(url)
	if strings.Contains(base, version) {
		return nil
	}
	return fmt.Errorf("indexer asset %q does not include version %q", base, version)
}

func hasMarkerFile(dir, name string) bool {
	path := filepath.Join(dir, name)
	_, err := os.Stat(path)
	return err == nil
}

func hasFileExtension(dir string, extensions []string) bool {
	if len(extensions) == 0 {
		return false
	}
	normalized := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		normalized[strings.ToLower(ext)] = struct{}{}
	}

	found, err := walkForExtension(dir, normalized)
	if err != nil {
		return false
	}
	return found
}

func walkForExtension(root string, extensions map[string]struct{}) (bool, error) {
	var found bool
	sentinel := errors.New("found")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, ok := extensions[ext]; ok {
			found = true
			return sentinel
		}
		return nil
	})
	if err != nil && !errors.Is(err, sentinel) {
		return false, err
	}
	return found, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".gestalt", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func addLanguage(languages *[]string, seen map[string]struct{}, language string) {
	if _, ok := seen[language]; ok {
		return
	}
	seen[language] = struct{}{}
	*languages = append(*languages, language)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
