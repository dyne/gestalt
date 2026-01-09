package main

import (
	"path/filepath"
	"testing"
)

func TestBuildTemporalDevConfig(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "temporal.log")
	databasePath := filepath.Join(tempDir, "temporal.db")
	dynamicConfigPath := filepath.Join(tempDir, "dynamicconfig.yaml")
	historyURI := "file://" + filepath.ToSlash(filepath.Join(tempDir, "archival", "history"))
	visibilityURI := "file://" + filepath.ToSlash(filepath.Join(tempDir, "archival", "visibility"))

	cfg, err := buildTemporalDevConfig(temporalDevConfigPaths{
		LogPath:               logPath,
		DatabasePath:          databasePath,
		DynamicConfigPath:     dynamicConfigPath,
		ArchivalHistoryURI:    historyURI,
		ArchivalVisibilityURI: visibilityURI,
	})
	if err != nil {
		t.Fatalf("buildTemporalDevConfig error: %v", err)
	}

	if cfg.Log.OutputFile != logPath {
		t.Fatalf("expected log output %q, got %q", logPath, cfg.Log.OutputFile)
	}
	if cfg.Log.Stdout {
		t.Fatal("expected temporal logs to stay off stdout")
	}
	if cfg.DynamicConfigClient == nil || cfg.DynamicConfigClient.Filepath != dynamicConfigPath {
		t.Fatalf("expected dynamic config path %q, got %#v", dynamicConfigPath, cfg.DynamicConfigClient)
	}

	defaultStore, ok := cfg.Persistence.DataStores["sqlite-default"]
	if !ok || defaultStore.SQL == nil {
		t.Fatal("expected sqlite-default datastore config")
	}
	if defaultStore.SQL.DatabaseName != databasePath {
		t.Fatalf("expected sqlite-default database %q, got %q", databasePath, defaultStore.SQL.DatabaseName)
	}

	visibilityStore, ok := cfg.Persistence.DataStores["sqlite-visibility"]
	if !ok || visibilityStore.SQL == nil {
		t.Fatal("expected sqlite-visibility datastore config")
	}
	if visibilityStore.SQL.DatabaseName != databasePath {
		t.Fatalf("expected sqlite-visibility database %q, got %q", databasePath, visibilityStore.SQL.DatabaseName)
	}

	if cfg.NamespaceDefaults.Archival.History.URI != historyURI {
		t.Fatalf("expected history archival URI %q, got %q", historyURI, cfg.NamespaceDefaults.Archival.History.URI)
	}
	if cfg.NamespaceDefaults.Archival.Visibility.URI != visibilityURI {
		t.Fatalf("expected visibility archival URI %q, got %q", visibilityURI, cfg.NamespaceDefaults.Archival.Visibility.URI)
	}
}
