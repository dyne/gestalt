package plan

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanPlansDirectory parses all .org files in a plans directory.
func ScanPlansDirectory(dir string) ([]PlanDocument, error) {
	target := strings.TrimSpace(dir)
	if target == "" {
		target = DefaultPlansDir()
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []PlanDocument{}, nil
		}
		return nil, err
	}

	documents := []PlanDocument{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isValidFilename(name) {
			continue
		}
		fullPath := filepath.Join(target, name)
		source, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		orgaDoc, err := ParseWithOrga(fullPath)
		if err != nil {
			return nil, err
		}
		documents = append(documents, TransformDocument(name, orgaDoc, string(source)))
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].Filename > documents[j].Filename
	})

	return documents, nil
}

func isValidFilename(name string) bool {
	if !strings.HasSuffix(name, ".org") {
		return false
	}
	if name != filepath.Base(name) {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return true
}
