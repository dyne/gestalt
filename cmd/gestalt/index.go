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
	ensureIndexer   = scip.EnsureIndexer
	runIndexer      = scip.RunIndexer
	mergeIndexes    = scip.MergeIndexes
	convertToSQLite = scip.ConvertToSQLite
	openIndex       = scip.OpenIndex
	buildMetadata   = scip.BuildMetadata
	saveMetadata    = scip.SaveMetadata
)

func indexCommand() {
	os.Exit(runIndexCommand(os.Args[2:], os.Stdout, os.Stderr))
}

func runIndexCommand(args []string, out io.Writer, errOut io.Writer) int {
	indexFlags := flag.NewFlagSet("index", flag.ContinueOnError)
	indexFlags.SetOutput(errOut)
	path := indexFlags.String("path", ".", "Path to repository")
	output := indexFlags.String("output", "index.db", "Output SQLite path")
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
		if _, err := os.Stat(*output); err == nil {
			if recent, age, err := recentIndexAge(*output, scipReindexRecentThreshold); err != nil {
				fmt.Fprintf(errOut, "Warning: Failed to read index metadata: %v\n", err)
			} else if recent {
				fmt.Fprintf(errOut, "Warning: Index was created %s ago. Use --force to re-index.\n", age.Round(time.Second))
				return 0
			}
			fmt.Fprintf(out, "Index exists at %s. Use --force to re-index.\n", *output)
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
		if _, err := ensureIndexer(lang); err != nil {
			fmt.Fprintf(errOut, "Warning: Failed to get indexer for %s: %v\n", lang, err)
			continue
		}
		supported = append(supported, lang)
	}
	if len(supported) == 0 {
		fmt.Fprintln(errOut, "No supported languages detected.")
		return 1
	}

	var scipIndexes []string
	var indexedLanguages []string
	for _, lang := range supported {
		fmt.Fprintf(out, "Indexing %s code...\n", lang)
		scipOut := filepath.Join(repoPath, fmt.Sprintf("index-%s.scip", lang))
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

	finalScip := scipIndexes[0]
	if len(scipIndexes) > 1 {
		finalScip = filepath.Join(repoPath, "index.scip")
		if err := mergeIndexes(scipIndexes, finalScip); err != nil {
			fmt.Fprintf(errOut, "Error merging indexes: %v\n", err)
			return 1
		}
	}

	outputDir := filepath.Dir(*output)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fmt.Fprintf(errOut, "Unable to create output directory: %v\n", err)
			return 1
		}
	}

	fmt.Fprintln(out, "Converting to SQLite...")
	if err := convertToSQLite(finalScip, *output); err != nil {
		fmt.Fprintf(errOut, "Error converting to SQLite: %v\n", err)
		return 1
	}

	projectRoot := repoPath
	if absRoot, err := filepath.Abs(repoPath); err == nil {
		projectRoot = absRoot
	}
	meta, err := buildMetadata(projectRoot, indexedLanguages)
	if err != nil {
		fmt.Fprintf(errOut, "Warning: Failed to build index metadata: %v\n", err)
	} else if err := saveMetadata(*output, meta); err != nil {
		fmt.Fprintf(errOut, "Warning: Failed to save index metadata: %v\n", err)
	}

	index, err := openIndex(*output)
	if err != nil {
		fmt.Fprintf(errOut, "Error opening SQLite index: %v\n", err)
		return 1
	}
	defer index.Close()

	stats, err := index.GetStats()
	if err != nil {
		fmt.Fprintf(errOut, "Error reading index stats: %v\n", err)
		return 1
	}

	fmt.Fprintln(out, "Indexing complete!")
	fmt.Fprintf(out, "  Documents: %d\n", stats.Documents)
	fmt.Fprintf(out, "  Symbols: %d\n", stats.Symbols)
	fmt.Fprintf(out, "  Occurrences: %d\n", stats.Occurrences)
	fmt.Fprintf(out, "  Index: %s\n", *output)
	return 0
}

func printIndexHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: gestalt index [options]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --path      Path to repository (default: current directory)")
	fmt.Fprintln(out, "  --output    Output SQLite path (default: index.db)")
	fmt.Fprintln(out, "  --force     Re-index even if index exists")
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
