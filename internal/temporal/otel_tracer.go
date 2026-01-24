package temporal

import (
	"context"
	"strings"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/interceptor"
)

const temporalOTelHeaderKey = "otel"

type temporalSpanContextKey struct{}

var temporalSpanKey = temporalSpanContextKey{}

type otelTracer struct {
	interceptor.BaseTracer
	tracer trace.Tracer
}

type otelTracerSpan struct {
	span trace.Span
}

func newOTelTracer() *otelTracer {
	return &otelTracer{
		tracer: otelapi.Tracer("gestalt/temporal"),
	}
}

func temporalTracingInterceptor() interceptor.Interceptor {
	return interceptor.NewTracingInterceptor(newOTelTracer())
}

func (t *otelTracer) Options() interceptor.TracerOptions {
	return interceptor.TracerOptions{
		SpanContextKey: temporalSpanKey,
		HeaderKey:      temporalOTelHeaderKey,
	}
}

func (t *otelTracer) UnmarshalSpan(data map[string]string) (interceptor.TracerSpanRef, error) {
	if len(data) == 0 {
		return nil, nil
	}
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier(data))
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return nil, nil
	}
	return spanContext, nil
}

func (t *otelTracer) MarshalSpan(span interceptor.TracerSpan) (map[string]string, error) {
	otelSpan, ok := span.(*otelTracerSpan)
	if !ok || otelSpan == nil {
		return nil, nil
	}
	spanContext := otelSpan.span.SpanContext()
	if !spanContext.IsValid() {
		return nil, nil
	}
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return map[string]string(carrier), nil
}

func (t *otelTracer) SpanFromContext(ctx context.Context) interceptor.TracerSpan {
	if ctx == nil {
		return nil
	}
	if span, ok := ctx.Value(temporalSpanKey).(interceptor.TracerSpan); ok {
		return span
	}
	otelSpan := trace.SpanFromContext(ctx)
	if otelSpan == nil {
		return nil
	}
	if !otelSpan.SpanContext().IsValid() {
		return nil
	}
	return &otelTracerSpan{span: otelSpan}
}

func (t *otelTracer) ContextWithSpan(ctx context.Context, span interceptor.TracerSpan) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if otelSpan, ok := span.(*otelTracerSpan); ok && otelSpan != nil {
		ctx = trace.ContextWithSpan(ctx, otelSpan.span)
	}
	return context.WithValue(ctx, temporalSpanKey, span)
}

func (t *otelTracer) StartSpan(options *interceptor.TracerStartSpanOptions) (interceptor.TracerSpan, error) {
	if options == nil {
		return nil, nil
	}
	ctx := context.Background()
	if options.Parent != nil {
		ctx = contextWithParent(ctx, options.Parent)
	}

	attrs := tagsToAttributes(options.Tags)
	spanName := t.SpanName(options)

	startOptions := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
	}
	if !options.Time.IsZero() {
		startOptions = append(startOptions, trace.WithTimestamp(options.Time))
	}
	if len(attrs) > 0 {
		startOptions = append(startOptions, trace.WithAttributes(attrs...))
	}

	ctx, span := t.tracer.Start(ctx, spanName, startOptions...)
	return &otelTracerSpan{span: span}, nil
}

func (s *otelTracerSpan) Finish(options *interceptor.TracerFinishSpanOptions) {
	if s == nil || s.span == nil {
		return
	}
	if options != nil && options.Error != nil {
		s.span.RecordError(options.Error)
		s.span.SetStatus(codes.Error, options.Error.Error())
	}
	s.span.End()
}

func contextWithParent(ctx context.Context, parent interceptor.TracerSpanRef) context.Context {
	switch typed := parent.(type) {
	case trace.SpanContext:
		if typed.IsValid() {
			return trace.ContextWithSpanContext(ctx, typed)
		}
	case *trace.SpanContext:
		if typed != nil && typed.IsValid() {
			return trace.ContextWithSpanContext(ctx, *typed)
		}
	case *otelTracerSpan:
		if typed != nil && typed.span != nil {
			return trace.ContextWithSpan(ctx, typed.span)
		}
	case otelTracerSpan:
		if typed.span != nil {
			return trace.ContextWithSpan(ctx, typed.span)
		}
	case trace.Span:
		if typed != nil {
			return trace.ContextWithSpan(ctx, typed)
		}
	}
	return ctx
}

func tagsToAttributes(tags map[string]string) []attribute.KeyValue {
	if len(tags) == 0 {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, len(tags))
	for key, value := range tags {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		attrs = append(attrs, attribute.String(trimmedKey, value))
	}
	return attrs
}
