package flow

import (
	"os"
	"path/filepath"
	"strings"
)

func listManagedFlowFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	managed := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, managedFlowSuffix) {
			managed = append(managed, name)
		}
	}
	return managed, nil
}

func removeStaleManagedFlowFiles(dir string, desired map[string]struct{}) error {
	managed, err := listManagedFlowFiles(dir)
	if err != nil {
		return err
	}
	for _, name := range managed {
		if _, ok := desired[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
