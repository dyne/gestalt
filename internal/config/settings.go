package config

import (
	"os"
	"strings"

	"gestalt/internal/config/tomlkeys"
)

type Settings struct {
	Session SessionSettings
}

type SessionSettings struct {
	LogMaxBytes           int64
	HistoryScanMaxBytes   int64
	ScrollbackLines       int64
	FontFamily            string
	FontSize              string
	InputFontFamily       string
	InputFontSize         string
	TUIMode               string
	TUISnapshotIntervalMS int64
	LogCodexEvents        bool
}

func LoadSettings(path string, defaultsPayload []byte, overrides map[string]any) (Settings, error) {
	defaultsStore, err := tomlkeys.Decode(defaultsPayload)
	if err != nil {
		return Settings{}, err
	}
	defaults := defaultsStore.Flat()
	values := defaultsStore.Flat()

	if strings.TrimSpace(path) != "" {
		payload, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return Settings{}, err
			}
		} else {
			store, err := tomlkeys.Decode(payload)
			if err != nil {
				return Settings{}, err
			}
			for key, value := range store.Flat() {
				values[key] = value
			}
		}
	}

	for key, value := range overrides {
		normalized := tomlkeys.NormalizeKey(key)
		if normalized == "" {
			continue
		}
		values[normalized] = value
	}

	settings := Settings{}

	settings.Session.LogMaxBytes = intSetting(values, "session.log-max-bytes", 0)
	settings.Session.HistoryScanMaxBytes = intSetting(values, "session.history-scan-max-bytes", 0)
	settings.Session.ScrollbackLines = intSetting(values, "session.scrollback-lines", 0)
	settings.Session.FontFamily = stringSetting(values, "session.font-family", "")
	settings.Session.FontSize = stringSetting(values, "session.font-size", "")
	settings.Session.InputFontFamily = stringSetting(values, "session.input-font-family", "")
	settings.Session.InputFontSize = stringSetting(values, "session.input-font-size", "")
	settings.Session.TUIMode = stringSetting(values, "session.tui-mode", "")
	settings.Session.TUISnapshotIntervalMS = intSetting(values, "session.tui-snapshot-interval-ms", 0)
	settings.Session.LogCodexEvents = boolSetting(values, "session.log-codex-events", boolSetting(defaults, "session.log-codex-events", false))

	return normalizeSettings(settings, defaults), nil
}

func normalizeSettings(settings Settings, defaults map[string]any) Settings {
	if settings.Session.LogMaxBytes <= 0 {
		settings.Session.LogMaxBytes = intSetting(defaults, "session.log-max-bytes", 0)
	}
	if settings.Session.HistoryScanMaxBytes <= 0 {
		settings.Session.HistoryScanMaxBytes = intSetting(defaults, "session.history-scan-max-bytes", 0)
	}
	if settings.Session.ScrollbackLines <= 0 {
		settings.Session.ScrollbackLines = intSetting(defaults, "session.scrollback-lines", 0)
	}
	if settings.Session.FontFamily == "" {
		settings.Session.FontFamily = stringSetting(defaults, "session.font-family", "")
	}
	if settings.Session.FontSize == "" {
		settings.Session.FontSize = stringSetting(defaults, "session.font-size", "")
	}
	if settings.Session.InputFontFamily == "" {
		settings.Session.InputFontFamily = stringSetting(defaults, "session.input-font-family", "")
	}
	if settings.Session.InputFontSize == "" {
		settings.Session.InputFontSize = stringSetting(defaults, "session.input-font-size", "")
	}
	if settings.Session.TUIMode == "" {
		settings.Session.TUIMode = stringSetting(defaults, "session.tui-mode", "")
	}
	if settings.Session.TUISnapshotIntervalMS <= 0 {
		settings.Session.TUISnapshotIntervalMS = intSetting(defaults, "session.tui-snapshot-interval-ms", 0)
	}
	return settings
}

func intSetting(values map[string]any, key string, fallback int64) int64 {
	value, ok := values[tomlkeys.NormalizeKey(key)]
	if !ok {
		return fallback
	}
	if parsed, ok := asInt64(value); ok {
		return parsed
	}
	return fallback
}

func stringSetting(values map[string]any, key string, fallback string) string {
	value, ok := values[tomlkeys.NormalizeKey(key)]
	if !ok {
		return fallback
	}
	if parsed, ok := value.(string); ok {
		return strings.TrimSpace(parsed)
	}
	return fallback
}

func boolSetting(values map[string]any, key string, fallback bool) bool {
	value, ok := values[tomlkeys.NormalizeKey(key)]
	if !ok {
		return fallback
	}
	if parsed, ok := value.(bool); ok {
		return parsed
	}
	return fallback
}

func asInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case uint64:
		return int64(typed), true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	case float32:
		if typed == float32(int64(typed)) {
			return int64(typed), true
		}
	}
	return 0, false
}
