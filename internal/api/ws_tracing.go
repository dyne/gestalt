package api

import (
	"context"
	"net/http"
	"strings"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const wsConnectSpanName = "websocket.connect"

func startWebSocketSpan(r *http.Request, route string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
		ctx = otelapi.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
	}

	tracer := otelapi.Tracer("gestalt/ws")
	baseAttrs := wsSpanAttributes(r, route)
	if len(attrs) > 0 {
		baseAttrs = append(baseAttrs, attrs...)
	}

	return tracer.Start(ctx, wsConnectSpanName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(baseAttrs...),
	)
}

func wsSpanAttributes(r *http.Request, route string) []attribute.KeyValue {
	attributes := make([]attribute.KeyValue, 0, 6)
	if r != nil {
		attributes = append(attributes,
			attribute.String("http.method", r.Method),
			attribute.String("http.target", sanitizeWSTarget(r)),
			attribute.String("http.scheme", wsRequestScheme(r)),
			attribute.String("user_agent", r.UserAgent()),
		)
	}
	if strings.TrimSpace(route) != "" {
		attributes = append(attributes, attribute.String("http.route", route))
	}
	return attributes
}

func wsRequestScheme(r *http.Request) string {
	if r == nil {
		return "http"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func sanitizeWSTarget(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	copyURL := *r.URL
	query := copyURL.Query()
	query.Del("token")
	copyURL.RawQuery = query.Encode()
	return copyURL.RequestURI()
}
