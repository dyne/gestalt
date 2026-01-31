package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gestalt"
	"gestalt/internal/config/tomlkeys"
	"gestalt/internal/logging"
)

func saveGestaltConfigDefaults(cfg Config, paths configPaths, logger *logging.Logger) {
	path := gestaltConfigPath(cfg, paths)
	defaultsPayload, err := fs.ReadFile(gestalt.EmbeddedConfigFS, filepath.Join("config", gestaltConfigFilename))
	if err != nil {
		logConfigSaveError(logger, "read embedded defaults failed", err)
		return
	}
	defaultsStore, err := tomlkeys.Decode(defaultsPayload)
	if err != nil {
		logConfigSaveError(logger, "parse embedded defaults failed", err)
		return
	}

	existingPayload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			payload, err := renderGestaltToml(defaultsStore.Flat())
			if err != nil {
				logConfigSaveError(logger, "render defaults failed", err)
				return
			}
			if err := writeFileAtomic(path, payload, 0o644); err != nil {
				logConfigSaveError(logger, "write defaults failed", err)
				return
			}
			logConfigSaveInfo(logger, path, true)
			return
		}
		logConfigSaveError(logger, "read config failed", err)
		return
	}

	existingStore, err := tomlkeys.Decode(existingPayload)
	if err != nil {
		logConfigSaveError(logger, "parse config failed", err)
		return
	}
	merged, missing := mergeGestaltConfig(defaultsStore.Flat(), existingStore.Flat())
	if !missing {
		return
	}
	payload, err := renderGestaltToml(merged)
	if err != nil {
		logConfigSaveError(logger, "render config failed", err)
		return
	}
	if err := writeFileAtomic(path, payload, 0o644); err != nil {
		logConfigSaveError(logger, "write config failed", err)
		return
	}
	logConfigSaveInfo(logger, path, false)
}

func mergeGestaltConfig(defaults, existing map[string]any) (map[string]any, bool) {
	merged := make(map[string]any)
	missing := false
	for key, value := range defaults {
		if existingValue, ok := existing[key]; ok {
			merged[key] = existingValue
			continue
		}
		merged[key] = value
		missing = true
	}
	for key, value := range existing {
		if _, ok := merged[key]; ok {
			continue
		}
		merged[key] = value
	}
	return merged, missing
}

func renderGestaltToml(values map[string]any) ([]byte, error) {
	categories := make(map[string]map[string]any)
	root := make(map[string]any)
	for key, value := range values {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) == 1 {
			root[parts[0]] = value
			continue
		}
		category := parts[0]
		field := parts[1]
		if _, ok := categories[category]; !ok {
			categories[category] = make(map[string]any)
		}
		categories[category][field] = value
	}

	var out bytes.Buffer
	if err := writeTomlSection(&out, "", root); err != nil {
		return nil, err
	}

	categoryNames := make([]string, 0, len(categories))
	for category := range categories {
		categoryNames = append(categoryNames, category)
	}
	sort.Strings(categoryNames)
	for _, category := range categoryNames {
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString("[")
		out.WriteString(category)
		out.WriteString("]\n")
		if err := writeTomlSection(&out, category, categories[category]); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func writeTomlSection(out *bytes.Buffer, prefix string, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := formatTomlValue(values[key])
		if err != nil {
			if prefix == "" {
				return fmt.Errorf("format %s: %w", key, err)
			}
			return fmt.Errorf("format %s.%s: %w", prefix, key, err)
		}
		out.WriteString(key)
		out.WriteString(" = ")
		out.WriteString(value)
		out.WriteString("\n")
	}
	return nil
}

func formatTomlValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed), nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case int:
		return strconv.FormatInt(int64(typed), 10), nil
	case int32:
		return strconv.FormatInt(int64(typed), 10), nil
	case int16:
		return strconv.FormatInt(int64(typed), 10), nil
	case int8:
		return strconv.FormatInt(int64(typed), 10), nil
	case uint64:
		return strconv.FormatUint(typed, 10), nil
	case uint:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), nil
	case time.Time:
		return typed.Format(time.RFC3339Nano), nil
	case []any:
		return formatTomlArray(typed)
	default:
		return "", fmt.Errorf("unsupported type %T", value)
	}
}

func formatTomlArray(values []any) (string, error) {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		formatted, err := formatTomlValue(value)
		if err != nil {
			return "", err
		}
		parts = append(parts, formatted)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

func writeFileAtomic(path string, payload []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".gestalt-config-")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)
	if _, err := tempFile.Write(payload); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, perm); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func logConfigSaveError(logger *logging.Logger, message string, err error) {
	if logger == nil {
		return
	}
	logger.Warn(message, map[string]string{
		"error": err.Error(),
	})
}

func logConfigSaveInfo(logger *logging.Logger, path string, isNew bool) {
	if logger == nil {
		return
	}
	action := "updated"
	if isNew {
		action = "created"
	}
	logger.Info("gestalt config defaults applied", map[string]string{
		"path":   path,
		"action": action,
	})
}
