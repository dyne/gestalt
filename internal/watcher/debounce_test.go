package watcher

import (
	"testing"
	"time"
)

func TestDebouncerCoalescesEvents(t *testing.T) {
	debouncer := newDebouncer(25 * time.Millisecond)
	defer debouncer.stop()

	received := make(chan string, 2)
	flush := func(path string) {
		received <- path
	}

	dropped := debouncer.schedule("path", Event{Path: "path"}, flush)
	if dropped {
		t.Fatalf("expected first event not to be dropped")
	}
	dropped = debouncer.schedule("path", Event{Path: "path"}, flush)
	if !dropped {
		t.Fatalf("expected second event to be coalesced")
	}

	count := 0
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case <-received:
			count++
		case <-deadline:
			if count != 1 {
				t.Fatalf("expected 1 flush, got %d", count)
			}
			return
		}
	}
}
