package shellgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type Entry struct {
	Key   string
	Value interface{}
}

func FlattenConfig(config map[string]interface{}) []Entry {
	entries := []Entry{}
	flattenMap("", config, &entries)
	return entries
}

func FormatValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", typed)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", typed)
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(payload)
}

func EscapeShellArg(value string) string {
	if value == "" {
		return "''"
	}
	if !needsQuoting(value) {
		return value
	}
	replacer := strings.NewReplacer("'", "'\"'\"'")
	return "'" + replacer.Replace(value) + "'"
}

func NormalizeFlag(name string) string {
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func flattenMap(prefix string, value interface{}, entries *[]Entry) {
	if value == nil {
		return
	}

	if list, ok := value.([]interface{}); ok {
		for _, item := range list {
			*entries = append(*entries, Entry{Key: prefix, Value: item})
		}
		return
	}

	if list, ok := value.([]string); ok {
		for _, item := range list {
			*entries = append(*entries, Entry{Key: prefix, Value: item})
		}
		return
	}

	mapValue, ok := asStringMap(value)
	if !ok {
		*entries = append(*entries, Entry{Key: prefix, Value: value})
		return
	}

	keys := make([]string, 0, len(mapValue))
	for key := range mapValue {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := mapValue[key]
		childKey := key
		if prefix != "" {
			childKey = prefix + "." + key
		}
		flattenMap(childKey, child, entries)
	}
}

func asStringMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	}
	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Map {
		return nil, false
	}
	if val.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	result := make(map[string]interface{}, val.Len())
	iter := val.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		result[key] = iter.Value().Interface()
	}
	return result, true
}

func needsQuoting(value string) bool {
	for _, r := range value {
		switch r {
		case ' ', '\t', '\n', '\r', '\'', '"', '\\', '$', '&', ';', '|', '>', '<', '(', ')', '*', '?', '[', ']', '{', '}', '!', '#':
			return true
		}
	}
	return false
}
