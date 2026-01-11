package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Registry struct {
	workflowStarted   atomic.Int64
	workflowCompleted atomic.Int64
	workflowFailed    atomic.Int64
	workflowPaused    atomic.Int64
	activities        sync.Map
	eventBuses        sync.Map
	eventTypes        sync.Map
}

type activityStats struct {
	count         atomic.Int64
	failures      atomic.Int64
	retries       atomic.Int64
	durationNanos atomic.Int64
}

type eventBusStats struct {
	subscribers atomic.Int64
}

type eventTypeStats struct {
	published atomic.Int64
	dropped   atomic.Int64
}

type eventTypeKey struct {
	bus       string
	eventType string
}

var Default = &Registry{}

func (r *Registry) IncWorkflowStarted() {
	if r == nil {
		return
	}
	r.workflowStarted.Add(1)
}

func (r *Registry) IncWorkflowCompleted() {
	if r == nil {
		return
	}
	r.workflowCompleted.Add(1)
}

func (r *Registry) IncWorkflowFailed() {
	if r == nil {
		return
	}
	r.workflowFailed.Add(1)
}

func (r *Registry) IncWorkflowPaused() {
	if r == nil {
		return
	}
	r.workflowPaused.Add(1)
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
}

func (r *Registry) IncEventPublished(busName, eventType string) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	eventType = normalizeMetricLabel(eventType, "unknown")
	stats := r.eventTypeStats(busName, eventType)
	stats.published.Add(1)
}

func (r *Registry) IncEventDropped(busName, eventType string) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	eventType = normalizeMetricLabel(eventType, "unknown")
	stats := r.eventTypeStats(busName, eventType)
	stats.dropped.Add(1)
}

func (r *Registry) SetEventSubscribers(busName string, count int) {
	if r == nil {
		return
	}
	busName = normalizeMetricLabel(busName, "unknown")
	if count < 0 {
		count = 0
	}
	stats := r.eventBusStats(busName)
	stats.subscribers.Store(int64(count))
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
		fmt.Fprintf(writer, "gestalt_event_subscribers{bus=%s} %d\n", formatLabel(busName), stats.subscribers.Load())
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
