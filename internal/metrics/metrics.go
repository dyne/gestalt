package metrics

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Registry struct {
	workflowStarted   atomic.Int64
	workflowCompleted atomic.Int64
	workflowFailed    atomic.Int64
	workflowPaused    atomic.Int64
	activities        sync.Map
	eventBuses        sync.Map
	eventTypes        sync.Map
	otelOnce          sync.Once
	otelMetrics       *otelRegistry
}

type activityStats struct {
	count         atomic.Int64
	failures      atomic.Int64
	retries       atomic.Int64
	durationNanos atomic.Int64
}

type eventBusStats struct {
	filtered   atomic.Int64
	unfiltered atomic.Int64
}

type eventTypeStats struct {
	published atomic.Int64
	dropped   atomic.Int64
}

type eventTypeKey struct {
	bus       string
	eventType string
}

type otelRegistry struct {
	workflowStarted   metric.Int64Counter
	workflowCompleted metric.Int64Counter
	workflowFailed    metric.Int64Counter
	workflowPaused    metric.Int64Counter

	activityDuration metric.Float64Histogram
	activityFailures metric.Int64Counter
	activityRetries  metric.Int64Counter

	eventPublished   metric.Int64Counter
	eventDropped     metric.Int64Counter
	eventSubscribers metric.Int64UpDownCounter
}

type EventBusSnapshot struct {
	Name                  string
	FilteredSubscribers   int64
	UnfilteredSubscribers int64
}

var Default = &Registry{}

func (r *Registry) IncWorkflowStarted() {
	if r == nil {
		return
	}
	r.workflowStarted.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.workflowStarted != nil {
		otelMetrics.workflowStarted.Add(context.Background(), 1)
	}
}

func (r *Registry) IncWorkflowCompleted() {
	if r == nil {
		return
	}
	r.workflowCompleted.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.workflowCompleted != nil {
		otelMetrics.workflowCompleted.Add(context.Background(), 1)
	}
}

func (r *Registry) IncWorkflowFailed() {
	if r == nil {
		return
	}
	r.workflowFailed.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.workflowFailed != nil {
		otelMetrics.workflowFailed.Add(context.Background(), 1)
	}
}

func (r *Registry) IncWorkflowPaused() {
	if r == nil {
		return
	}
	r.workflowPaused.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.workflowPaused != nil {
		otelMetrics.workflowPaused.Add(context.Background(), 1)
	}
}

func (r *Registry) RecordActivity(name string, duration time.Duration, err error, attempt int32) {
	if r == nil {
		return
	}
	if strings.TrimSpace(name) == "" {
		name = "unknown"
	}
	stats := r.activityStats(name)
	stats.count.Add(1)
	stats.durationNanos.Add(duration.Nanoseconds())
	if err != nil {
		stats.failures.Add(1)
	}
	if attempt > 1 {
		stats.retries.Add(1)
	}

	otelMetrics := r.otel()
	if otelMetrics == nil {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("activity.name", name)}
	ctx := context.Background()
	if otelMetrics.activityDuration != nil {
		otelMetrics.activityDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	}
	if err != nil && otelMetrics.activityFailures != nil {
		otelMetrics.activityFailures.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if attempt > 1 && otelMetrics.activityRetries != nil {
		otelMetrics.activityRetries.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (r *Registry) IncEventPublished(busName, eventType string) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	eventType = normalizeMetricLabel(eventType, "unknown")
	stats := r.eventTypeStats(busName, eventType)
	stats.published.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.eventPublished != nil {
		attrs := []attribute.KeyValue{
			attribute.String("event.bus", busName),
			attribute.String("event.type", eventType),
		}
		otelMetrics.eventPublished.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

func (r *Registry) IncEventDropped(busName, eventType string) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	eventType = normalizeMetricLabel(eventType, "unknown")
	stats := r.eventTypeStats(busName, eventType)
	stats.dropped.Add(1)
	if otelMetrics := r.otel(); otelMetrics != nil && otelMetrics.eventDropped != nil {
		attrs := []attribute.KeyValue{
			attribute.String("event.bus", busName),
			attribute.String("event.type", eventType),
		}
		otelMetrics.eventDropped.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

func (r *Registry) SetEventSubscriberCounts(busName string, filtered, unfiltered int) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	if filtered < 0 {
		filtered = 0
	}
	if unfiltered < 0 {
		unfiltered = 0
	}
	stats := r.eventBusStats(busName)
	prevFiltered := stats.filtered.Swap(int64(filtered))
	prevUnfiltered := stats.unfiltered.Swap(int64(unfiltered))

	otelMetrics := r.otel()
	if otelMetrics != nil && otelMetrics.eventSubscribers != nil {
		ctx := context.Background()
		filteredDelta := int64(filtered) - prevFiltered
		if filteredDelta != 0 {
			attrs := []attribute.KeyValue{
				attribute.String("event.bus", busName),
				attribute.Bool("event.filtered", true),
			}
			otelMetrics.eventSubscribers.Add(ctx, filteredDelta, metric.WithAttributes(attrs...))
		}
		unfilteredDelta := int64(unfiltered) - prevUnfiltered
		if unfilteredDelta != 0 {
			attrs := []attribute.KeyValue{
				attribute.String("event.bus", busName),
				attribute.Bool("event.filtered", false),
			}
			otelMetrics.eventSubscribers.Add(ctx, unfilteredDelta, metric.WithAttributes(attrs...))
		}
	}
}

func (r *Registry) WritePrometheus(writer io.Writer) error {
	if r == nil {
		return nil
	}

	writeCounter(writer, "gestalt_workflows_started_total", "Total workflows started", r.workflowStarted.Load())
	writeCounter(writer, "gestalt_workflows_completed_total", "Total workflows completed", r.workflowCompleted.Load())
	writeCounter(writer, "gestalt_workflows_failed_total", "Total workflows failed", r.workflowFailed.Load())
	writeCounter(writer, "gestalt_workflows_paused_total", "Total workflow pauses", r.workflowPaused.Load())

	eventBusNames := r.eventBusNames()
	sort.Strings(eventBusNames)
	eventTypeKeys := r.eventTypeKeys()
	sort.Slice(eventTypeKeys, func(i, j int) bool {
		if eventTypeKeys[i].bus == eventTypeKeys[j].bus {
			return eventTypeKeys[i].eventType < eventTypeKeys[j].eventType
		}
		return eventTypeKeys[i].bus < eventTypeKeys[j].bus
	})

	writeHelp(writer, "gestalt_events_published_total", "Total events published")
	fmt.Fprintln(writer, "# TYPE gestalt_events_published_total counter")
	writeHelp(writer, "gestalt_events_dropped_total", "Total events dropped")
	fmt.Fprintln(writer, "# TYPE gestalt_events_dropped_total counter")
	writeHelp(writer, "gestalt_event_subscribers", "Active event bus subscribers")
	fmt.Fprintln(writer, "# TYPE gestalt_event_subscribers gauge")

	for _, key := range eventTypeKeys {
		stats := r.eventTypeStats(key.bus, key.eventType)
		busLabel := formatLabel(key.bus)
		typeLabel := formatLabel(key.eventType)
		fmt.Fprintf(writer, "gestalt_events_published_total{bus=%s,type=%s} %d\n", busLabel, typeLabel, stats.published.Load())
		fmt.Fprintf(writer, "gestalt_events_dropped_total{bus=%s,type=%s} %d\n", busLabel, typeLabel, stats.dropped.Load())
	}

	for _, busName := range eventBusNames {
		stats := r.eventBusStats(busName)
		label := formatLabel(busName)
		fmt.Fprintf(writer, "gestalt_event_subscribers{bus=%s,filtered=\"true\"} %d\n", label, stats.filtered.Load())
		fmt.Fprintf(writer, "gestalt_event_subscribers{bus=%s,filtered=\"false\"} %d\n", label, stats.unfiltered.Load())
	}

	activityNames := r.activityNames()
	sort.Strings(activityNames)

	writeHelp(writer, "gestalt_activity_duration_seconds", "Activity duration in seconds")
	fmt.Fprintln(writer, "# TYPE gestalt_activity_duration_seconds summary")
	writeHelp(writer, "gestalt_activity_failures_total", "Activity failures")
	fmt.Fprintln(writer, "# TYPE gestalt_activity_failures_total counter")
	writeHelp(writer, "gestalt_activity_retries_total", "Activity retries")
	fmt.Fprintln(writer, "# TYPE gestalt_activity_retries_total counter")

	for _, name := range activityNames {
		stats := r.activityStats(name)
		label := formatLabel(name)
		durationSeconds := float64(stats.durationNanos.Load()) / float64(time.Second)
		fmt.Fprintf(writer, "gestalt_activity_duration_seconds_sum{activity=%s} %.6f\n", label, durationSeconds)
		fmt.Fprintf(writer, "gestalt_activity_duration_seconds_count{activity=%s} %d\n", label, stats.count.Load())
		fmt.Fprintf(writer, "gestalt_activity_failures_total{activity=%s} %d\n", label, stats.failures.Load())
		fmt.Fprintf(writer, "gestalt_activity_retries_total{activity=%s} %d\n", label, stats.retries.Load())
	}

	return nil
}

func (r *Registry) EventBusSnapshots() []EventBusSnapshot {
	if r == nil {
		return nil
	}
	names := r.eventBusNames()
	sort.Strings(names)
	snapshots := make([]EventBusSnapshot, 0, len(names))
	for _, name := range names {
		stats := r.eventBusStats(name)
		snapshots = append(snapshots, EventBusSnapshot{
			Name:                  name,
			FilteredSubscribers:   stats.filtered.Load(),
			UnfilteredSubscribers: stats.unfiltered.Load(),
		})
	}
	return snapshots
}

func (r *Registry) activityStats(name string) *activityStats {
	value, _ := r.activities.LoadOrStore(name, &activityStats{})
	return value.(*activityStats)
}

func (r *Registry) eventBusStats(name string) *eventBusStats {
	value, _ := r.eventBuses.LoadOrStore(name, &eventBusStats{})
	return value.(*eventBusStats)
}

func (r *Registry) eventTypeStats(busName, eventType string) *eventTypeStats {
	key := eventTypeKey{bus: busName, eventType: eventType}
	value, _ := r.eventTypes.LoadOrStore(key, &eventTypeStats{})
	return value.(*eventTypeStats)
}

func (r *Registry) activityNames() []string {
	if r == nil {
		return nil
	}
	var names []string
	r.activities.Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			names = append(names, name)
		}
		return true
	})
	return names
}

func (r *Registry) eventBusNames() []string {
	if r == nil {
		return nil
	}
	var names []string
	r.eventBuses.Range(func(key, value interface{}) bool {
		if name, ok := key.(string); ok {
			names = append(names, name)
		}
		return true
	})
	return names
}

func (r *Registry) eventTypeKeys() []eventTypeKey {
	if r == nil {
		return nil
	}
	var keys []eventTypeKey
	r.eventTypes.Range(func(key, value interface{}) bool {
		if entry, ok := key.(eventTypeKey); ok {
			keys = append(keys, entry)
		}
		return true
	})
	return keys
}

func (r *Registry) otel() *otelRegistry {
	if r == nil {
		return nil
	}
	r.otelOnce.Do(func() {
		r.otelMetrics = newOTelRegistry()
	})
	return r.otelMetrics
}

func newOTelRegistry() *otelRegistry {
	meter := otelapi.GetMeterProvider().Meter("gestalt/metrics")
	registry := &otelRegistry{}

	registry.workflowStarted, _ = meter.Int64Counter(
		"gestalt.workflow.started",
		metric.WithDescription("Total workflows started"),
	)
	registry.workflowCompleted, _ = meter.Int64Counter(
		"gestalt.workflow.completed",
		metric.WithDescription("Total workflows completed"),
	)
	registry.workflowFailed, _ = meter.Int64Counter(
		"gestalt.workflow.failed",
		metric.WithDescription("Total workflows failed"),
	)
	registry.workflowPaused, _ = meter.Int64Counter(
		"gestalt.workflow.paused",
		metric.WithDescription("Total workflow pauses"),
	)

	registry.activityDuration, _ = meter.Float64Histogram(
		"gestalt.activity.duration",
		metric.WithDescription("Activity duration"),
		metric.WithUnit("s"),
	)
	registry.activityFailures, _ = meter.Int64Counter(
		"gestalt.activity.failures",
		metric.WithDescription("Activity failures"),
	)
	registry.activityRetries, _ = meter.Int64Counter(
		"gestalt.activity.retries",
		metric.WithDescription("Activity retries"),
	)

	registry.eventPublished, _ = meter.Int64Counter(
		"gestalt.event.published",
		metric.WithDescription("Total events published"),
	)
	registry.eventDropped, _ = meter.Int64Counter(
		"gestalt.event.dropped",
		metric.WithDescription("Total events dropped"),
	)
	registry.eventSubscribers, _ = meter.Int64UpDownCounter(
		"gestalt.event.subscribers",
		metric.WithDescription("Active event bus subscribers"),
	)

	return registry
}

func writeHelp(writer io.Writer, metric, help string) {
	fmt.Fprintf(writer, "# HELP %s %s\n", metric, help)
}

func writeCounter(writer io.Writer, metric, help string, value int64) {
	writeHelp(writer, metric, help)
	fmt.Fprintf(writer, "# TYPE %s counter\n", metric)
	fmt.Fprintf(writer, "%s %d\n", metric, value)
}

func formatLabel(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

func normalizeMetricLabel(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
