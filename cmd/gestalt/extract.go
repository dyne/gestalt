package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gestalt"
)

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func runExtractConfig() int {
	configFS, err := fs.Sub(gestalt.EmbeddedConfigFS, "config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract config failed: %v\n", err)
		return 1
	}
	distFS, err := fs.Sub(gestalt.EmbeddedFrontendFS, path.Join("frontend", "dist"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract frontend failed: %v\n", err)
		return 1
	}

	destRoot := "gestalt"
	configDest := filepath.Join(destRoot, "config")
	distDest := filepath.Join(destRoot, "dist")
	if info, err := os.Stat(destRoot); err == nil && !info.IsDir() {
		fmt.Fprintf(os.Stderr, "extract config failed: %s exists and is not a directory\n", destRoot)
		return 1
	}
	if err := os.MkdirAll(configDest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create config directory failed: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(distDest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create dist directory failed: %v\n", err)
		return 1
	}

	var agentsCount int
	var promptsCount int
	var skillsCount int
	var frontendCount int

	if err := extractEmbeddedTree(configFS, configDest, func(entry string) {
		switch {
		case strings.HasPrefix(entry, "agents/") && strings.HasSuffix(entry, ".json"):
			agentsCount++
		case strings.HasPrefix(entry, "prompts/") && strings.HasSuffix(entry, ".txt"):
			promptsCount++
		case strings.HasPrefix(entry, "skills/") && path.Base(entry) == "SKILL.md":
			skillsCount++
		}
	}); err != nil {
		fmt.Fprintf(os.Stderr, "extract config failed: %v\n", err)
		return 1
	}

	if err := extractEmbeddedTree(distFS, distDest, func(entry string) {
		frontendCount++
	}); err != nil {
		fmt.Fprintf(os.Stderr, "extract frontend failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Extracted %d agents, %d prompts, %d skills, %d frontend assets to ./gestalt/\n",
		agentsCount, promptsCount, skillsCount, frontendCount)
	return 0
}

func extractEmbeddedTree(src fs.FS, destRoot string, record func(string)) error {
	return fs.WalkDir(src, ".", func(entry string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry == "." {
			return nil
		}
		destPath := filepath.Join(destRoot, filepath.FromSlash(entry))
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}
		if _, err := os.Stat(destPath); err == nil {
			fmt.Fprintf(os.Stderr, "warning: %s already exists, skipping\n", destPath)
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := fs.ReadFile(src, entry)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if info, err := d.Info(); err == nil {
			if mode := info.Mode().Perm(); mode != 0 {
				perm = mode
			}
		}
		if err := os.WriteFile(destPath, data, perm); err != nil {
			return err
		}
		if record != nil {
			record(entry)
		}
		return nil
	})
}
