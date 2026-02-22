package schema

// ActualType returns the schema-style type name for the provided value.
func ActualType(value any) string {
	return actualType(value)
}

// AsStringMap coerces map-like values into a map keyed by strings.
func AsStringMap(value any) (map[string]any, bool) {
	return asStringMap(value)
}

// FormatActualDetail formats a human-readable actual type detail for errors.
func FormatActualDetail(actualType string, value any) string {
	return formatActualDetail(actualType, value)
}

// FormatValidationValue formats a value for display in validation errors.
func FormatValidationValue(value any) string {
	return formatValidationValue(value)
}
