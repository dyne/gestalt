//go:build !noscip

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gestalt/internal/scip"
)

const scipReindexRecentThreshold = 10 * time.Minute

var (
	detectLanguages = scip.DetectLanguages
	ensureIndexer   = scip.EnsureIndexerAtPath
	runIndexer      = scip.RunIndexer
	mergeIndexes    = scip.MergeIndexes
	buildMetadata   = scip.BuildMetadata
	saveMetadata    = scip.SaveMetadata
)

func runIndexCommand(args []string, out io.Writer, errOut io.Writer) int {
	indexFlags := flag.NewFlagSet("index", flag.ContinueOnError)
	indexFlags.SetOutput(errOut)
	path := indexFlags.String("path", ".", "Path to repository")
	output := indexFlags.String("output", filepath.Join(".gestalt", "scip", "index.scip"), "Output .scip path")
	force := indexFlags.Bool("force", false, "Re-index even if index exists")
	help := indexFlags.Bool("help", false, "Show help")
	helpShort := indexFlags.Bool("h", false, "Show help")

	indexFlags.Usage = func() {
		printIndexHelp(errOut)
	}

	if err := indexFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printIndexHelp(out)
			return 0
		}
		return 1
	}
	if *help || *helpShort {
		printIndexHelp(out)
		return 0
	}

	repoPath := strings.TrimSpace(*path)
	if repoPath == "" {
		fmt.Fprintln(errOut, "Path is required.")
		return 1
	}
	outputPath := strings.TrimSpace(*output)
	if outputPath == "" {
		fmt.Fprintln(errOut, "Output path is required.")
		return 1
	}
	outputFile, scipDir, outputErr := resolveScipOutput(outputPath)
	if outputErr != nil {
		fmt.Fprintf(errOut, "Output path error: %v\n", outputErr)
		return 1
	}
	info, err := os.Stat(repoPath)
	if err != nil {
		fmt.Fprintf(errOut, "Path not found: %v\n", err)
		return 1
	}
	if !info.IsDir() {
		fmt.Fprintf(errOut, "Path is not a directory: %s\n", repoPath)
		return 1
	}

	if !*force {
		if _, err := os.Stat(outputFile); err == nil {
			if recent, age, err := recentIndexAge(outputFile, scipReindexRecentThreshold); err != nil {
				fmt.Fprintf(errOut, "Warning: Failed to read index metadata: %v\n", err)
			} else if recent {
				fmt.Fprintf(errOut, "Warning: Index was created %s ago. Use --force to re-index.\n", age.Round(time.Second))
				return 0
			}
			fmt.Fprintf(out, "Index exists at %s. Use --force to re-index.\n", outputFile)
			return 0
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(errOut, "Unable to access output path: %v\n", err)
			return 1
		}
	}

	languages, err := detectLanguages(repoPath)
	if err != nil {
		fmt.Fprintf(errOut, "Failed to detect languages: %v\n", err)
		return 1
	}
	if len(languages) == 0 {
		fmt.Fprintln(errOut, "No supported languages detected.")
		return 1
	}

	fmt.Fprintf(out, "Detected languages: %v\n", languages)

	var supported []string
	for _, lang := range languages {
		fmt.Fprintf(out, "Checking indexer for %s...\n", lang)
		if _, err := ensureIndexer(lang, repoPath); err != nil {
			fmt.Fprintf(errOut, "Warning: Failed to get indexer for %s: %v\n", lang, err)
			continue
		}
		supported = append(supported, lang)
	}
	if len(supported) == 0 {
		fmt.Fprintln(errOut, "No supported languages detected.")
		return 1
	}

	if scipDir != "." {
		if err := os.MkdirAll(scipDir, 0o755); err != nil {
			fmt.Fprintf(errOut, "Unable to create output directory: %v\n", err)
			return 1
		}
	}

	var scipIndexes []string
	var indexedLanguages []string
	for _, lang := range supported {
		fmt.Fprintf(out, "Indexing %s code...\n", lang)
		scipOut := filepath.Join(scipDir, fmt.Sprintf("index-%s.scip", lang))
		if err := runIndexer(lang, repoPath, scipOut); err != nil {
			fmt.Fprintf(errOut, "Warning: Indexing %s failed: %v\n", lang, err)
			continue
		}
		scipIndexes = append(scipIndexes, scipOut)
		indexedLanguages = append(indexedLanguages, lang)
	}
	if len(scipIndexes) == 0 {
		fmt.Fprintln(errOut, "No indexes were generated.")
		return 1
	}

	if err := buildMergedIndex(scipIndexes, outputFile); err != nil {
		fmt.Fprintf(errOut, "Error writing merged index: %v\n", err)
		return 1
	}

	projectRoot := repoPath
	if absRoot, err := filepath.Abs(repoPath); err == nil {
		projectRoot = absRoot
	}
	meta, err := buildMetadata(projectRoot, indexedLanguages)
	if err != nil {
		fmt.Fprintf(errOut, "Warning: Failed to build index metadata: %v\n", err)
	} else if err := saveMetadata(outputFile, meta); err != nil {
		fmt.Fprintf(errOut, "Warning: Failed to save index metadata: %v\n", err)
	}

	fmt.Fprintln(out, "Indexing complete!")
	fmt.Fprintf(out, "  Index: %s\n", outputFile)
	return 0
}

func printIndexHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt index [options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --path      Path to repository (default: current directory)")
	fmt.Fprintln(out, "  --output    Output .scip path (default: .gestalt/scip/index.scip)")
	fmt.Fprintln(out, "  --force     Re-index even if index exists")
}

func resolveScipOutput(outputPath string) (string, string, error) {
	cleaned := strings.TrimSpace(outputPath)
	if cleaned == "" {
		return "", "", fmt.Errorf("output path is required")
	}

	if info, err := os.Stat(cleaned); err == nil {
		if info.IsDir() {
			outputFile := filepath.Join(cleaned, "index.scip")
			return outputFile, cleaned, nil
		}
		if filepath.Ext(cleaned) != ".scip" {
			return "", "", fmt.Errorf("output path must end with .scip or be a directory")
		}
		return cleaned, filepath.Dir(cleaned), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	ext := filepath.Ext(cleaned)
	if ext == "" {
		dir := strings.TrimSuffix(cleaned, string(os.PathSeparator))
		if dir == "" {
			dir = "."
		}
		return filepath.Join(dir, "index.scip"), dir, nil
	}
	if ext != ".scip" {
		return "", "", fmt.Errorf("output path must end with .scip or be a directory")
	}
	return cleaned, filepath.Dir(cleaned), nil
}

func buildMergedIndex(inputs []string, outputPath string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no scip indexes to merge")
	}
	if len(inputs) == 1 && inputs[0] == outputPath {
		return nil
	}
	tempPath := outputPath + ".tmp"
	_ = os.Remove(tempPath)

	if len(inputs) == 1 {
		payload, err := os.ReadFile(inputs[0])
		if err != nil {
			return fmt.Errorf("read scip index %s: %w", inputs[0], err)
		}
		if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
			return fmt.Errorf("write scip index %s: %w", tempPath, err)
		}
	} else {
		if err := mergeIndexes(inputs, tempPath); err != nil {
			return err
		}
	}

	if err := os.Rename(tempPath, outputPath); err == nil {
		return nil
	}
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing file: %w", err)
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func recentIndexAge(indexPath string, threshold time.Duration) (bool, time.Duration, error) {
	meta, err := scip.LoadMetadata(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if meta.CreatedAt.IsZero() {
		return false, 0, nil
	}
	age := time.Since(meta.CreatedAt)
	if age < 0 {
		age = 0
	}
	return age < threshold, age, nil
}
