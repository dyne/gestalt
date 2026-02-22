package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"

	"gestalt/internal/agent/schemas"
	internalschema "gestalt/internal/schema"
)

// ValidationError preserves agent error compatibility while using shared schema validation.
type ValidationError = internalschema.ValidationError

func ValidateAgentConfig(cliType string, config map[string]interface{}) error {
	if len(config) == 0 {
		return nil
	}
	s, err := schemas.SchemaFor(cliType)
	if err != nil {
		return err
	}
	return validateConfigWithSchema(s, config)
}

func validateConfigWithSchema(s *jsonschema.Schema, config map[string]interface{}) error {
	if s == nil {
		return nil
	}
	return internalschema.ValidateObject(s, config)
}

func formatActualDetail(actualType string, value any) string {
	actualType = strings.TrimSpace(actualType)
	if value == nil {
		return actualType
	}
	formatted := formatValidationValue(value)
	if formatted == "" {
		return actualType
	}
	if actualType == "" {
		return formatted
	}
	return fmt.Sprintf("%s (%s)", actualType, formatted)
}

func formatValidationValue(value any) string {
	if value == nil {
		return ""
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	text := string(payload)
	const maxLength = 160
	if len(text) > maxLength {
		return text[:maxLength-3] + "..."
	}
	return text
}

func actualType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float32, float64:
		return "number"
	case int, int8, int16, int32, int64:
		return "integer"
	case uint, uint8, uint16, uint32, uint64:
		return "integer"
	case json.Number:
		return "number"
	}
	if _, ok := asStringMap(value); ok {
		return "object"
	}
	return fmt.Sprintf("%T", value)
}

func asStringMap(value any) (map[string]interface{}, bool) {
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
