package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"gestalt"
	"gestalt/internal/config"
)

const (
	defaultConfigDir = ".gestalt/config"
	configRoot       = "config"
)

type overlayFS struct {
	primary  fs.FS
	fallback fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if o.primary != nil {
		file, err := o.primary.Open(name)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if o.fallback == nil {
		return nil, fs.ErrNotExist
	}
	return o.fallback.Open(name)
}

func ensureExtractedConfig(destDir string, in io.Reader, out io.Writer) error {
	info, err := os.Stat(destDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("config path is not a directory: %s", destDir)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat config dir %s: %w", destDir, err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", destDir, err)
	}

	extractor := config.Extractor{
		BackupLimit: 1,
		Resolver: &config.ConffileResolver{
			Interactive: stdinIsInteractive(),
			In:          in,
			Out:         out,
			DiffRunner:  conffileDiffRunner(),
		},
	}
	_, err = extractor.ExtractWithStats(gestalt.EmbeddedConfigFS, destDir, nil)
	if err != nil {
		return fmt.Errorf("extract config into %s: %w", destDir, err)
	}
	return nil
}

func buildConfigOverlay(destDir string) (fs.FS, string) {
	fallbackRoot := filepath.Dir(destDir)
	return overlayFS{
		primary:  os.DirFS("."),
		fallback: os.DirFS(fallbackRoot),
	}, configRoot
}

func stdinIsInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func conffileDiffRunner() config.DiffRunner {
	return func(oldPath, newPath string) (string, error) {
		cmd := exec.Command("diff", "-u", oldPath, newPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return string(output), nil
			}
			return "", err
		}
		return string(output), nil
	}
}
