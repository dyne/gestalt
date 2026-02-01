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
		ScrollbackLines       int64  `toml:"scrollback-lines"`
		FontFamily            string `toml:"font-family"`
		FontSize              string `toml:"font-size"`
		InputFontFamily       string `toml:"input-font-family"`
		InputFontSize         string `toml:"input-font-size"`
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
	if defaults.Session.ScrollbackLines != 2000 {
		t.Fatalf("expected scrollback-lines 2000, got %d", defaults.Session.ScrollbackLines)
	}
	if defaults.Session.FontFamily != "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, \"Liberation Mono\", \"Courier New\", monospace" {
		t.Fatalf("expected font-family default, got %q", defaults.Session.FontFamily)
	}
	if defaults.Session.FontSize != "0.95rem" {
		t.Fatalf("expected font-size 0.95rem, got %q", defaults.Session.FontSize)
	}
	if defaults.Session.InputFontFamily != "\"IBM Plex Mono\", \"JetBrains Mono\", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, \"Liberation Mono\", \"Courier New\", monospace" {
		t.Fatalf("expected input-font-family default, got %q", defaults.Session.InputFontFamily)
	}
	if defaults.Session.InputFontSize != "0.95rem" {
		t.Fatalf("expected input-font-size 0.95rem, got %q", defaults.Session.InputFontSize)
	}
	if defaults.Session.TUIMode != "" {
		t.Fatalf("expected tui-mode empty, got %q", defaults.Session.TUIMode)
	}
	if defaults.Session.TUISnapshotIntervalMS != 0 {
		t.Fatalf("expected tui-snapshot-interval-ms 0, got %d", defaults.Session.TUISnapshotIntervalMS)
	}
	if defaults.Temporal.MaxOutputBytes != 4096 {
		t.Fatalf("expected temporal.max-output-bytes 4096, got %d", defaults.Temporal.MaxOutputBytes)
	}
}
