package flow

import (
	"encoding/hex"
	"hash/fnv"
	"sort"
	"strings"
)

type EventDeduper struct {
	limit int
	order []string
	seen  map[string]struct{}
}

func NewEventDeduper(limit int) *EventDeduper {
	if limit <= 0 {
		limit = 1
	}
	return &EventDeduper{
		limit: limit,
		order: make([]string, 0, limit),
		seen:  make(map[string]struct{}, limit),
	}
}

func (deduper *EventDeduper) Seen(id string) bool {
	if deduper == nil {
		return false
	}
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return false
	}
	if _, ok := deduper.seen[trimmed]; ok {
		return true
	}
	deduper.seen[trimmed] = struct{}{}
	deduper.order = append(deduper.order, trimmed)
	for len(deduper.order) > deduper.limit {
		evict := deduper.order[0]
		deduper.order = deduper.order[1:]
		delete(deduper.seen, evict)
	}
	return false
}

func BuildEventID(normalized map[string]string) string {
	if len(normalized) == 0 {
		return ""
	}
	keys := make([]string, 0, len(normalized))
	for key := range normalized {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hasher := fnv.New64a()
	for _, key := range keys {
		_, _ = hasher.Write([]byte(key))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(normalized[key]))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func BuildIdempotencyKey(eventID, triggerID, activityID string) string {
	eventID = strings.TrimSpace(eventID)
	triggerID = strings.TrimSpace(triggerID)
	activityID = strings.TrimSpace(activityID)
	if eventID == "" && triggerID == "" && activityID == "" {
		return ""
	}
	return strings.Join([]string{eventID, triggerID, activityID}, "/")
}

type ActivityHeartbeat struct {
	Sent   bool `json:"sent"`
	Posted bool `json:"posted"`
}

func ShouldSkipSend(state *ActivityHeartbeat) bool {
	return state != nil && state.Sent
}

func ShouldSkipWebhook(state *ActivityHeartbeat) bool {
	return state != nil && state.Posted
}
