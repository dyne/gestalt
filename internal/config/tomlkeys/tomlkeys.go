package tomlkeys

import (
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Store struct {
	raw  map[string]any
	flat map[string]any
}

func (s Store) Flat() map[string]any {
	flat := make(map[string]any, len(s.flat))
	for key, value := range s.flat {
		flat[key] = value
	}
	return flat
}

func DecodeMap(data []byte) (map[string]any, error) {
	raw := map[string]any{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func Decode(data []byte) (Store, error) {
	raw, err := DecodeMap(data)
	if err != nil {
		return Store{}, err
	}
	return FromRaw(raw), nil
}

func FromRaw(raw map[string]any) Store {
	flat := make(map[string]any)
	flattenMap("", raw, flat)

	normalized := make(map[string]any, len(flat))
	keys := make([]string, 0, len(flat))
	for key := range flat {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		normalizedKey := NormalizeKey(key)
		if _, exists := normalized[normalizedKey]; exists {
			continue
		}
		normalized[normalizedKey] = flat[key]
	}

	return Store{raw: raw, flat: normalized}
}

func (s Store) GetBool(key string) (bool, bool) {
	value, ok := s.flat[NormalizeKey(key)]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	default:
		return false, false
	}
}

func (s Store) GetInt(key string) (int64, bool) {
	value, ok := s.flat[NormalizeKey(key)]
	if !ok {
		return 0, false
	}
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
	default:
		return 0, false
	}
}

func (s Store) GetString(key string) (string, bool) {
	value, ok := s.flat[NormalizeKey(key)]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	default:
		return "", false
	}
}

func NormalizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.Split(key, ".")
	for i, part := range parts {
		lowered := strings.ToLower(part)
		parts[i] = strings.ReplaceAll(lowered, "_", "-")
	}
	return strings.Join(parts, ".")
}

func flattenMap(prefix string, raw map[string]any, out map[string]any) {
	for key, value := range raw {
		flattenValue(joinKey(prefix, key), value, out)
	}
}

func flattenValue(key string, value any, out map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		flattenMap(key, typed, out)
	default:
		out[key] = value
	}
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
