package otel

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const defaultServiceName = "gestalt"

// SDKOptions configures the OpenTelemetry SDK exporters and resources.
type SDKOptions struct {
	Enabled            bool
	HTTPEndpoint       string
	ServiceName        string
	ServiceVersion     string
	ResourceAttributes map[string]string
}

func SDKOptionsFromEnv(stateDir string) SDKOptions {
	collector := OptionsFromEnv(stateDir)
	enabled := collector.Enabled
	if rawEnabled, ok := os.LookupEnv("GESTALT_OTEL_SDK_ENABLED"); ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(rawEnabled)); err == nil {
			enabled = parsed
		}
	}
	serviceName := strings.TrimSpace(os.Getenv("GESTALT_OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	resourceAttributes := parseResourceAttributes(os.Getenv("GESTALT_OTEL_RESOURCE_ATTRIBUTES"))
	return SDKOptions{
		Enabled:            enabled,
		HTTPEndpoint:       collector.HTTPEndpoint,
		ServiceName:        serviceName,
		ResourceAttributes: resourceAttributes,
	}
}

func SetupSDK(ctx context.Context, options SDKOptions) (func(context.Context) error, error) {
	if !options.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	endpoint := normalizeEndpoint(options.HTTPEndpoint)
	if endpoint == "" {
		endpoint = defaultHTTPEndpoint
	}

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return nil, err
	}

	resourceAttrs := []attribute.KeyValue{
		attribute.String("service.name", options.ServiceName),
	}
	if strings.TrimSpace(options.ServiceVersion) != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("service.version", options.ServiceVersion))
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("host.name", host))
	}
	for key, value := range options.ResourceAttributes {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		resourceAttrs = append(resourceAttrs, attribute.String(trimmedKey, value))
	}

	res, err := sdkresource.New(ctx, sdkresource.WithAttributes(resourceAttrs...))
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		_ = metricExporter.Shutdown(ctx)
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)

	otelapi.SetTracerProvider(tracerProvider)
	otelapi.SetMeterProvider(meterProvider)
	otelapi.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(shutdownCtx context.Context) error {
		var shutdownErr error
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		if err := meterProvider.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		return shutdownErr
	}, nil
}

func parseResourceAttributes(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	attributes := make(map[string]string)
	pairs := strings.Split(trimmed, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		attributes[key] = value
	}
	if len(attributes) == 0 {
		return nil
	}
	return attributes
}

func normalizeEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	endpoint = strings.TrimSuffix(endpoint, "/")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	return strings.TrimSuffix(endpoint, "/")
}
