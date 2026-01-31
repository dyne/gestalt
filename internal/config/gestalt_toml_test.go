package config

import (
	"io/fs"
	"testing"

	"gestalt"
	"github.com/BurntSushi/toml"
)

type gestaltDefaults struct {
	Session struct {
		LogMaxBytes           int64  `toml:"log-max-bytes"`
		HistoryScanMaxBytes   int64  `toml:"history-scan-max-bytes"`
		TUIMode               string `toml:"tui-mode"`
		TUISnapshotIntervalMS int64  `toml:"tui-snapshot-interval-ms"`
	} `toml:"session"`
	Temporal struct {
		MaxOutputBytes int64 `toml:"max-output-bytes"`
	} `toml:"temporal"`
}

func TestEmbeddedGestaltTomlDefaults(t *testing.T) {
	payload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, "config/gestalt.toml")
	if err != nil {
		t.Fatalf("read embedded gestalt.toml: %v", err)
	}

	var defaults gestaltDefaults
	if _, err := toml.Decode(string(payload), &defaults); err != nil {
		t.Fatalf("decode gestalt.toml: %v", err)
	}

	if defaults.Session.LogMaxBytes != 5*1024*1024 {
		t.Fatalf("expected log-max-bytes 5242880, got %d", defaults.Session.LogMaxBytes)
	}
	if defaults.Session.HistoryScanMaxBytes != 2*1024*1024 {
		t.Fatalf("expected history-scan-max-bytes 2097152, got %d", defaults.Session.HistoryScanMaxBytes)
	}
	if defaults.Session.TUIMode != "snapshot" {
		t.Fatalf("expected tui-mode snapshot, got %q", defaults.Session.TUIMode)
	}
	if defaults.Session.TUISnapshotIntervalMS != 1000 {
		t.Fatalf("expected tui-snapshot-interval-ms 1000, got %d", defaults.Session.TUISnapshotIntervalMS)
	}
	if defaults.Temporal.MaxOutputBytes != 4096 {
		t.Fatalf("expected temporal.max-output-bytes 4096, got %d", defaults.Temporal.MaxOutputBytes)
	}
}
