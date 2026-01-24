package temporal

import (
	"context"
	"testing"
	"time"

	otelapi "go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/interceptor"
)

func TestOTelTracerMarshalUnmarshal(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otelapi.GetTracerProvider()
	otelapi.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otelapi.SetTracerProvider(previous)
	})

	tracer := newOTelTracer()
	span, err := tracer.StartSpan(&interceptor.TracerStartSpanOptions{
		Operation: "workflow",
		Name:      "SessionWorkflow",
		Time:      time.Now().UTC(),
		Tags: map[string]string{
			"workflow.id": "wf-1",
		},
	})
	if err != nil {
		t.Fatalf("StartSpan error: %v", err)
	}

	payload, err := tracer.MarshalSpan(span)
	if err != nil {
		t.Fatalf("MarshalSpan error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("expected payload")
	}

	ref, err := tracer.UnmarshalSpan(payload)
	if err != nil {
		t.Fatalf("UnmarshalSpan error: %v", err)
	}
	if ref == nil {
		t.Fatalf("expected span ref")
	}
	if spanRef, ok := ref.(trace.SpanContext); !ok || !spanRef.IsValid() {
		t.Fatalf("expected valid span context")
	}

	span.(*otelTracerSpan).Finish(nil)
}

func TestOTelTracerStartSpanUsesParent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otelapi.GetTracerProvider()
	otelapi.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otelapi.SetTracerProvider(previous)
	})

	parentTracer := otelapi.Tracer("test")
	_, parentSpan := parentTracer.Start(context.Background(), "parent")
	parentContext := parentSpan.SpanContext()
	parentSpan.End()

	tracer := newOTelTracer()
	span, err := tracer.StartSpan(&interceptor.TracerStartSpanOptions{
		Parent:    parentContext,
		Operation: "activity",
		Name:      "SpawnTerminal",
	})
	if err != nil {
		t.Fatalf("StartSpan error: %v", err)
	}

	otelSpan := span.(*otelTracerSpan)
	if otelSpan.span.SpanContext().TraceID() != parentContext.TraceID() {
		t.Fatalf("expected child span to share trace ID")
	}
	otelSpan.Finish(nil)
}
