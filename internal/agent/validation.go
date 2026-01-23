package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"

	"gestalt/internal/agent/schemas"
)

type ValidationError struct {
	Path        string
	Expected    string
	Actual      string
	ActualValue any
	Message     string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" && e.Expected == "" && e.Actual == "" {
		if e.Path == "" {
			return e.Message
		}
		return fmt.Sprintf("%s: %s", e.Path, e.Message)
	}
	actualDetail := formatActualDetail(e.Actual, e.ActualValue)
	if e.Path == "" {
		return fmt.Sprintf("expected %s, got %s", e.Expected, actualDetail)
	}
	return fmt.Sprintf("%s: expected %s, got %s", e.Path, e.Expected, actualDetail)
}

func ValidateAgentConfig(cliType string, config map[string]interface{}) error {
	if len(config) == 0 {
		return nil
	}
	schema, err := schemas.SchemaFor(cliType)
	if err != nil {
		return err
	}
	return validateConfigWithSchema(schema, config)
}

func validateConfigWithSchema(schema *jsonschema.Schema, config map[string]interface{}) error {
	if schema == nil {
		return nil
	}
	return validateSchema(schema, config, "")
}

func validateSchema(schema *jsonschema.Schema, value any, path string) error {
	if schema == nil {
		return nil
	}
	if value == nil {
		if schemaAllowsNull(schema) {
			return nil
		}
		return &ValidationError{
			Path:     path,
			Expected: expectedType(schema),
			Actual:   "null",
		}
	}

	if len(schema.AnyOf) > 0 {
		for _, option := range schema.AnyOf {
			if validateSchema(option, value, path) == nil {
				return nil
			}
		}
		return &ValidationError{
			Path:     path,
			Expected: expectedType(schema),
			Actual:   actualType(value),
			ActualValue: value,
		}
	}

	if len(schema.OneOf) > 0 {
		validCount := 0
		for _, option := range schema.OneOf {
			if validateSchema(option, value, path) == nil {
				validCount++
			}
		}
		if validCount == 1 {
			return nil
		}
		return &ValidationError{
			Path:     path,
			Expected: expectedType(schema),
			Actual:   actualType(value),
			ActualValue: value,
		}
	}

	switch resolvedType(schema) {
	case "object":
		object, ok := asStringMap(value)
		if !ok {
			return &ValidationError{
				Path:     path,
				Expected: "object",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		return validateObject(schema, object, path)
	case "array":
		array, ok := asSlice(value)
		if !ok {
			return &ValidationError{
				Path:     path,
				Expected: "array",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		if schema.Items == nil {
			return nil
		}
		for index, entry := range array {
			entryPath := fmt.Sprintf("%s[%d]", path, index)
			if path == "" {
				entryPath = fmt.Sprintf("[%d]", index)
			}
			if err := validateSchema(schema.Items, entry, entryPath); err != nil {
				return err
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return &ValidationError{
				Path:     path,
				Expected: "string",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		return validateEnum(schema, value, path)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return &ValidationError{
				Path:     path,
				Expected: "boolean",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		return nil
	case "integer":
		if !isInteger(value) {
			return &ValidationError{
				Path:     path,
				Expected: "integer",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		return nil
	case "number":
		if !isNumber(value) {
			return &ValidationError{
				Path:     path,
				Expected: "number",
				Actual:   actualType(value),
				ActualValue: value,
			}
		}
		return nil
	case "null":
		return &ValidationError{
			Path:     path,
			Expected: "null",
			Actual:   actualType(value),
			ActualValue: value,
		}
	default:
		return nil
	}
}

func validateObject(schema *jsonschema.Schema, object map[string]interface{}, path string) error {
	for _, required := range schema.Required {
		if _, ok := object[required]; !ok {
			requiredPath := joinPath(path, required)
			return &ValidationError{
				Path:    requiredPath,
				Message: "missing required field",
			}
		}
	}

	propertySchemas := map[string]*jsonschema.Schema{}
	if schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			propertySchemas[pair.Key] = pair.Value
		}
	}

	for key, value := range object {
		propertyPath := joinPath(path, key)
		if propertySchema, ok := propertySchemas[key]; ok {
			if err := validateSchema(propertySchema, value, propertyPath); err != nil {
				return err
			}
			continue
		}
		if schema.AdditionalProperties != nil {
			if isFalseSchema(schema.AdditionalProperties) {
				return &ValidationError{
					Path:    propertyPath,
					Message: "unknown field",
					ActualValue: value,
				}
			}
			if err := validateSchema(schema.AdditionalProperties, value, propertyPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateEnum(schema *jsonschema.Schema, value any, path string) error {
	if len(schema.Enum) == 0 {
		return nil
	}
	for _, candidate := range schema.Enum {
		if reflect.DeepEqual(candidate, value) {
			return nil
		}
	}
	return &ValidationError{
		Path:     path,
		Expected: "enum",
		Actual:   actualType(value),
		ActualValue: value,
	}
}

func resolvedType(schema *jsonschema.Schema) string {
	if schema == nil {
		return ""
	}
	if schema.Type != "" {
		return schema.Type
	}
	if schema.Properties != nil || schema.AdditionalProperties != nil {
		return "object"
	}
	if schema.Items != nil || len(schema.PrefixItems) > 0 {
		return "array"
	}
	return ""
}

func schemaAllowsNull(schema *jsonschema.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.Type == "null" {
		return true
	}
	for _, option := range schema.AnyOf {
		if resolvedType(option) == "null" {
			return true
		}
	}
	for _, option := range schema.OneOf {
		if resolvedType(option) == "null" {
			return true
		}
	}
	return false
}

func expectedType(schema *jsonschema.Schema) string {
	if schema == nil {
		return "unknown"
	}
	if schema.Type != "" {
		return schema.Type
	}
	types := []string{}
	for _, option := range schema.AnyOf {
		if t := resolvedType(option); t != "" {
			types = append(types, t)
		}
	}
	for _, option := range schema.OneOf {
		if t := resolvedType(option); t != "" {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		return "unknown"
	}
	return strings.Join(types, " or ")
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
	if _, ok := asSlice(value); ok {
		return "array"
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

func asSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []interface{}:
		return typed, true
	}
	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, false
	}
	result := make([]any, val.Len())
	for i := 0; i < val.Len(); i++ {
		result[i] = val.Index(i).Interface()
	}
	return result, true
}

func joinPath(base string, field string) string {
	if base == "" {
		return field
	}
	return base + "." + field
}

func isFalseSchema(schema *jsonschema.Schema) bool {
	if schema == nil {
		return false
	}
	marshaled, err := json.Marshal(schema)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(marshaled)) == "false"
}

func isInteger(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return typed == float32(int64(typed))
	case float64:
		return typed == float64(int64(typed))
	case json.Number:
		if _, err := typed.Int64(); err == nil {
			return true
		}
	}
	return false
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	case json.Number:
		return true
	}
	return false
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
