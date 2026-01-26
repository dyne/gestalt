package otel

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

type logHubProcessor struct {
	hub              *LogHub
	fallbackResource map[string]any
}

func newLogHubProcessor(hub *LogHub, fallbackResource map[string]any) sdklog.Processor {
	if hub == nil {
		return nil
	}
	return &logHubProcessor{
		hub:              hub,
		fallbackResource: fallbackResource,
	}
}

func (processor *logHubProcessor) OnEmit(_ context.Context, record *sdklog.Record) error {
	if processor == nil || processor.hub == nil || record == nil {
		return nil
	}
	otlp := sdkRecordToOTLP(record, processor.fallbackResource)
	if otlp != nil {
		processor.hub.Append(otlp)
	}
	return nil
}

func (processor *logHubProcessor) Shutdown(context.Context) error {
	return nil
}

func (processor *logHubProcessor) ForceFlush(context.Context) error {
	return nil
}

func sdkRecordToOTLP(record *sdklog.Record, fallbackResource map[string]any) map[string]any {
	if record == nil {
		return nil
	}

	body := otlpAnyValueFromLog(record.Body())
	if body == nil {
		body = map[string]any{}
	}

	attributes := otlpAttributesFromLog(record)
	resourceMap := otlpResourceFromSDK(record.Resource(), fallbackResource)
	scopeMap := otlpScopeFromSDK(record.InstrumentationScope())

	payload := map[string]any{
		"timeUnixNano":         formatUnixNano(record.Timestamp()),
		"observedTimeUnixNano": formatUnixNano(record.ObservedTimestamp()),
		"severityNumber":       int(record.Severity()),
		"severityText":         severityText(record),
		"body":                 body,
		"attributes":           attributes,
		"resource":             resourceMap,
		"scope":                scopeMap,
	}

	return payload
}

func formatUnixNano(timestamp time.Time) string {
	if timestamp.IsZero() {
		return ""
	}
	return strconv.FormatInt(timestamp.UnixNano(), 10)
}

func severityText(record *sdklog.Record) string {
	if record == nil {
		return ""
	}
	text := record.SeverityText()
	if text == "" && record.Severity() != otellog.SeverityUndefined {
		return record.Severity().String()
	}
	return text
}

func otlpAttributesFromLog(record *sdklog.Record) []any {
	if record == nil || record.AttributesLen() == 0 {
		return []any{}
	}
	attributes := make([]any, 0, record.AttributesLen())
	record.WalkAttributes(func(kv otellog.KeyValue) bool {
		if kv.Key == "" {
			return true
		}
		value := otlpAnyValueFromLog(kv.Value)
		if value == nil {
			return true
		}
		attributes = append(attributes, map[string]any{
			"key":   kv.Key,
			"value": value,
		})
		return true
	})
	return attributes
}

func otlpAnyValueFromLog(value otellog.Value) map[string]any {
	switch value.Kind() {
	case otellog.KindString:
		return map[string]any{"stringValue": value.AsString()}
	case otellog.KindBool:
		return map[string]any{"boolValue": value.AsBool()}
	case otellog.KindInt64:
		return map[string]any{"intValue": strconv.FormatInt(value.AsInt64(), 10)}
	case otellog.KindFloat64:
		return map[string]any{"doubleValue": value.AsFloat64()}
	case otellog.KindBytes:
		return map[string]any{"bytesValue": base64.StdEncoding.EncodeToString(value.AsBytes())}
	case otellog.KindSlice:
		values := value.AsSlice()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			if convertedValue := otlpAnyValueFromLog(entry); convertedValue != nil {
				converted = append(converted, convertedValue)
			}
		}
		return map[string]any{"arrayValue": map[string]any{"values": converted}}
	case otellog.KindMap:
		values := value.AsMap()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			if entry.Key == "" {
				continue
			}
			convertedValue := otlpAnyValueFromLog(entry.Value)
			if convertedValue == nil {
				continue
			}
			converted = append(converted, map[string]any{
				"key":   entry.Key,
				"value": convertedValue,
			})
		}
		return map[string]any{"kvlistValue": map[string]any{"values": converted}}
	default:
		return nil
	}
}

func otlpResourceFromSDK(res *resource.Resource, fallback map[string]any) map[string]any {
	if res == nil {
		return resourceFallback(fallback)
	}
	attrs := otlpAttributesFromAttributes(res.Attributes())
	if len(attrs) == 0 && res.SchemaURL() == "" {
		return resourceFallback(fallback)
	}
	resourceMap := map[string]any{
		"attributes": attrs,
	}
	if schemaURL := res.SchemaURL(); schemaURL != "" {
		resourceMap["schemaUrl"] = schemaURL
	}
	return resourceMap
}

func otlpResourceFromAttributes(attrs []attribute.KeyValue) map[string]any {
	return map[string]any{"attributes": otlpAttributesFromAttributes(attrs)}
}

func otlpScopeFromSDK(scope instrumentation.Scope) map[string]any {
	scopeMap := map[string]any{}
	if scope.Name != "" {
		scopeMap["name"] = scope.Name
	}
	if scope.Version != "" {
		scopeMap["version"] = scope.Version
	}
	if scope.SchemaURL != "" {
		scopeMap["schemaUrl"] = scope.SchemaURL
	}
	if attributes := otlpAttributesFromAttributes(scope.Attributes.ToSlice()); len(attributes) > 0 {
		scopeMap["attributes"] = attributes
	}
	return scopeMap
}

func otlpAttributesFromAttributes(attrs []attribute.KeyValue) []any {
	if len(attrs) == 0 {
		return []any{}
	}
	converted := make([]any, 0, len(attrs))
	for _, entry := range attrs {
		if entry.Key == "" {
			continue
		}
		value := otlpAnyValueFromAttribute(entry.Value)
		if value == nil {
			continue
		}
		converted = append(converted, map[string]any{
			"key":   entry.Key,
			"value": value,
		})
	}
	return converted
}

func otlpAnyValueFromAttribute(value attribute.Value) map[string]any {
	switch value.Type() {
	case attribute.BOOL:
		return map[string]any{"boolValue": value.AsBool()}
	case attribute.INT64:
		return map[string]any{"intValue": strconv.FormatInt(value.AsInt64(), 10)}
	case attribute.FLOAT64:
		return map[string]any{"doubleValue": value.AsFloat64()}
	case attribute.STRING:
		return map[string]any{"stringValue": value.AsString()}
	case attribute.BOOLSLICE:
		values := value.AsBoolSlice()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			converted = append(converted, map[string]any{"boolValue": entry})
		}
		return map[string]any{"arrayValue": map[string]any{"values": converted}}
	case attribute.INT64SLICE:
		values := value.AsInt64Slice()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			converted = append(converted, map[string]any{"intValue": strconv.FormatInt(entry, 10)})
		}
		return map[string]any{"arrayValue": map[string]any{"values": converted}}
	case attribute.FLOAT64SLICE:
		values := value.AsFloat64Slice()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			converted = append(converted, map[string]any{"doubleValue": entry})
		}
		return map[string]any{"arrayValue": map[string]any{"values": converted}}
	case attribute.STRINGSLICE:
		values := value.AsStringSlice()
		converted := make([]any, 0, len(values))
		for _, entry := range values {
			converted = append(converted, map[string]any{"stringValue": entry})
		}
		return map[string]any{"arrayValue": map[string]any{"values": converted}}
	default:
		return nil
	}
}

func resourceFallback(fallback map[string]any) map[string]any {
	if fallback != nil {
		return fallback
	}
	return map[string]any{"attributes": []any{}}
}
