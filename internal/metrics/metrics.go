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
}

type activityStats struct {
	count         atomic.Int64
	failures      atomic.Int64
	retries       atomic.Int64
	durationNanos atomic.Int64
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

func (r *Registry) WritePrometheus(writer io.Writer) error {
	if r == nil {
		return nil
	}

	writeCounter(writer, "gestalt_workflows_started_total", "Total workflows started", r.workflowStarted.Load())
	writeCounter(writer, "gestalt_workflows_completed_total", "Total workflows completed", r.workflowCompleted.Load())
	writeCounter(writer, "gestalt_workflows_failed_total", "Total workflows failed", r.workflowFailed.Load())
	writeCounter(writer, "gestalt_workflows_paused_total", "Total workflow pauses", r.workflowPaused.Load())

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
