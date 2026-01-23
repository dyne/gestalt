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
	flattenMap("", config, &entries, true)
	return entries
}

func FlattenConfigPreserveArrays(config map[string]interface{}) []Entry {
	entries := []Entry{}
	flattenMap("", config, &entries, false)
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

func flattenMap(prefix string, value interface{}, entries *[]Entry, expandArrays bool) {
	if isEmptyValue(value) {
		return
	}

	if list, ok := value.([]interface{}); ok {
		if expandArrays {
			for _, item := range list {
				if isEmptyValue(item) {
					continue
				}
				*entries = append(*entries, Entry{Key: prefix, Value: item})
			}
			return
		}
		*entries = append(*entries, Entry{Key: prefix, Value: list})
		return
	}

	if list, ok := value.([]string); ok {
		if expandArrays {
			for _, item := range list {
				if strings.TrimSpace(item) == "" {
					continue
				}
				*entries = append(*entries, Entry{Key: prefix, Value: item})
			}
			return
		}
		*entries = append(*entries, Entry{Key: prefix, Value: list})
		return
	}

	mapValue, ok := asStringMap(value)
	if !ok {
		*entries = append(*entries, Entry{Key: prefix, Value: value})
		return
	}

	if len(mapValue) == 0 {
		return
	}
	keys := make([]string, 0, len(mapValue))
	for key := range mapValue {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child := mapValue[key]
		if isEmptyValue(child) {
			continue
		}
		childKey := key
		if prefix != "" {
			childKey = prefix + "." + key
		}
		flattenMap(childKey, child, entries, expandArrays)
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

func isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []interface{}:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	}
	if mapValue, ok := asStringMap(value); ok {
		return len(mapValue) == 0
	}
	return false
}
