package otel

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

const defaultLogHubRetention = 0
const defaultLogHubBufferSize = 256

// DefaultLogHubMaxRecords caps the number of log records retained for snapshots.
const DefaultLogHubMaxRecords = 1000

var activeLogHub = NewLogHub(defaultLogHubRetention)

type LogHub struct {
	mu          sync.Mutex
	retention   time.Duration
	maxRecords  int
	records     []logHubRecord
	subscribers map[int]chan map[string]any
	nextID      int
}

type logHubRecord struct {
	timestamp time.Time
	record    map[string]any
}

func NewLogHub(retention time.Duration) *LogHub {
	if retention < 0 {
		retention = 0
	}
	return &LogHub{
		retention:   retention,
		maxRecords:  DefaultLogHubMaxRecords,
		records:     make([]logHubRecord, 0, 256),
		subscribers: make(map[int]chan map[string]any),
	}
}

func ActiveLogHub() *LogHub {
	return activeLogHub
}

// SetActiveLogHubMaxRecords updates the active log hub retention by count.
func SetActiveLogHubMaxRecords(maxRecords int) {
	hub := ActiveLogHub()
	if hub == nil {
		return
	}
	hub.SetMaxRecords(maxRecords)
}

func SetActiveLogHub(hub *LogHub) {
	if hub == nil {
		hub = NewLogHub(defaultLogHubRetention)
	}
	activeLogHub = hub
}

// MaxRecords returns the current max record cap for snapshots.
func (hub *LogHub) MaxRecords() int {
	if hub == nil {
		return 0
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return hub.maxRecords
}

// SetMaxRecords updates the snapshot record cap, falling back to the default.
func (hub *LogHub) SetMaxRecords(maxRecords int) {
	if hub == nil {
		return
	}
	if maxRecords <= 0 {
		maxRecords = DefaultLogHubMaxRecords
	}
	hub.mu.Lock()
	hub.maxRecords = maxRecords
	hub.pruneLocked(time.Time{})
	hub.mu.Unlock()
}

func (hub *LogHub) Append(records ...map[string]any) {
	if hub == nil || len(records) == 0 {
		return
	}
	now := time.Now()
	cutoff := time.Time{}
	if hub.retention > 0 {
		cutoff = now.Add(-hub.retention)
	}
	pending := make([]map[string]any, 0, len(records))

	hub.mu.Lock()
	for _, record := range records {
		if record == nil {
			continue
		}
		recordTime := logRecordTime(record)
		if recordTime.IsZero() {
			recordTime = now
		}
		if !cutoff.IsZero() && recordTime.Before(cutoff) {
			continue
		}
		hub.records = append(hub.records, logHubRecord{timestamp: recordTime, record: record})
		pending = append(pending, record)
	}
	hub.pruneLocked(cutoff)

	if len(pending) > 0 {
		for _, record := range pending {
			for _, subscriber := range hub.subscribers {
				select {
				case subscriber <- record:
				default:
				}
			}
		}
	}
	hub.mu.Unlock()
}

func (hub *LogHub) SnapshotSince(since time.Time) []map[string]any {
	if hub == nil {
		return nil
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	cutoff := time.Time{}
	if hub.retention > 0 {
		cutoff = time.Now().Add(-hub.retention)
	}
	hub.pruneLocked(cutoff)

	snapshot := make([]map[string]any, 0, len(hub.records))
	for _, entry := range hub.records {
		if !since.IsZero() && entry.timestamp.Before(since) {
			continue
		}
		snapshot = append(snapshot, entry.record)
	}
	return snapshot
}

func (hub *LogHub) Subscribe() (<-chan map[string]any, func()) {
	if hub == nil {
		ch := make(chan map[string]any)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan map[string]any, defaultLogHubBufferSize)
	hub.mu.Lock()
	hub.nextID++
	id := hub.nextID
	hub.subscribers[id] = ch
	hub.mu.Unlock()

	cancel := func() {
		hub.mu.Lock()
		if subscriber, ok := hub.subscribers[id]; ok {
			delete(hub.subscribers, id)
			close(subscriber)
		}
		hub.mu.Unlock()
	}

	return ch, cancel
}

func (hub *LogHub) pruneLocked(cutoff time.Time) {
	if len(hub.records) == 0 {
		return
	}
	if !cutoff.IsZero() {
		kept := hub.records[:0]
		for _, entry := range hub.records {
			if entry.timestamp.Before(cutoff) {
				continue
			}
			kept = append(kept, entry)
		}
		hub.records = kept
	}
	if hub.maxRecords > 0 && len(hub.records) > hub.maxRecords {
		start := len(hub.records) - hub.maxRecords
		trimmed := make([]logHubRecord, hub.maxRecords)
		copy(trimmed, hub.records[start:])
		hub.records = trimmed
	}
}

func logRecordTime(record map[string]any) time.Time {
	if record == nil {
		return time.Time{}
	}
	if parsed := parseUnixNano(record["timeUnixNano"]); !parsed.IsZero() {
		return parsed
	}
	return parseUnixNano(record["observedTimeUnixNano"])
}

func parseUnixNano(value any) time.Time {
	if value == nil {
		return time.Time{}
	}
	switch typed := value.(type) {
	case int64:
		return time.Unix(0, typed)
	case int:
		return time.Unix(0, int64(typed))
	case float64:
		return time.Unix(0, int64(typed))
	case json.Number:
		if parsed, err := strconv.ParseInt(string(typed), 10, 64); err == nil {
			return time.Unix(0, parsed)
		}
	case string:
		if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return time.Unix(0, parsed)
		}
	}
	return time.Time{}
}
