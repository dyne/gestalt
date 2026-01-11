package event

import (
	"context"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gestalt/internal/metrics"
)

const defaultSubscriberBufferSize = 128
const defaultDropWarningThreshold = 0.01
const defaultDropWarningInterval = 30 * time.Second

type BusOptions struct {
	Name                    string
	SubscriberBufferSize    int
	BlockOnFull             bool
	WriteTimeout            time.Duration
	MaxSubscribers          int
	SlowSubscriberThreshold time.Duration
	DropWarningThreshold    float64
	DropWarningInterval     time.Duration
	HistorySize             int
	Registry                *metrics.Registry
}

type Bus[T any] struct {
	mu           sync.Mutex
	subscribers  map[uint64]subscription[T]
	nextSubID    uint64
	closed       bool
	closeOnce    sync.Once
	options      BusOptions
	registry     *metrics.Registry
	published    atomic.Int64
	dropped      atomic.Int64
	lastWarning  atomic.Int64
	history      []T
	historyNext  int
	historyCount int
}

type typedEvent interface {
	Type() string
}

func NewBus[T any](ctx context.Context, opts BusOptions) *Bus[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.SubscriberBufferSize <= 0 {
		opts.SubscriberBufferSize = defaultSubscriberBufferSize
	}
	if opts.DropWarningThreshold <= 0 {
		opts.DropWarningThreshold = defaultDropWarningThreshold
	}
	if opts.DropWarningInterval <= 0 {
		opts.DropWarningInterval = defaultDropWarningInterval
	}
	bus := &Bus[T]{
		subscribers: make(map[uint64]subscription[T]),
		options:     opts,
		registry:    opts.Registry,
	}
	if opts.HistorySize > 0 {
		bus.history = make([]T, opts.HistorySize)
	}
	if bus.registry == nil {
		bus.registry = metrics.Default
	}
	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			bus.Close()
		}()
	}
	return bus
}

func (b *Bus[T]) Subscribe() (<-chan T, func()) {
	return b.SubscribeFiltered(nil)
}

func (b *Bus[T]) SubscribeFiltered(filter func(T) bool) (<-chan T, func()) {
	if b == nil {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan T, b.options.SubscriberBufferSize)
	id := atomic.AddUint64(&b.nextSubID, 1)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	if b.options.MaxSubscribers > 0 && len(b.subscribers) >= b.options.MaxSubscribers {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.subscribers[id] = subscription[T]{id: id, ch: ch, filter: filter}
	filtered, unfiltered := b.countSubscribersLocked()
	b.mu.Unlock()

	b.setSubscriberCounts(filtered, unfiltered)

	cancel := func() {
		b.removeSubscriber(id)
	}

	return ch, cancel
}

func (b *Bus[T]) SubscribeType(eventType string) (<-chan T, func()) {
	return b.SubscribeTypes(eventType)
}

func (b *Bus[T]) SubscribeTypes(eventTypes ...string) (<-chan T, func()) {
	if len(eventTypes) == 0 {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	typeSet := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		if eventType == "" {
			continue
		}
		typeSet[eventType] = struct{}{}
	}
	if len(typeSet) == 0 {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}

	filter := func(event T) bool {
		typed, ok := any(event).(Event)
		if !ok {
			return false
		}
		_, matched := typeSet[typed.Type()]
		return matched
	}

	return b.SubscribeFiltered(filter)
}

func (b *Bus[T]) Publish(event T) {
	if b == nil {
		return
	}
	if isNil(event) {
		return
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.appendHistoryLocked(event)
	if len(b.subscribers) == 0 {
		b.mu.Unlock()
		eventType := b.eventType(event)
		b.incPublished(eventType)
		if debugEventsEnabled {
			log.Printf("event bus %s: event %s", b.busName(), eventType)
		}
		return
	}
	subscribers := make([]subscription[T], 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		subscribers = append(subscribers, sub)
	}
	b.mu.Unlock()

	eventType := b.eventType(event)
	b.incPublished(eventType)
	if debugEventsEnabled {
		log.Printf("event bus %s: event %s", b.busName(), eventType)
	}

	for _, sub := range subscribers {
		if !b.filterAllows(sub, event) {
			continue
		}
		b.sendToSubscriber(sub, event, eventType)
	}
}

func (b *Bus[T]) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		subscribers := b.subscribers
		b.subscribers = make(map[uint64]subscription[T])
		b.mu.Unlock()

		for _, sub := range subscribers {
			close(sub.ch)
		}
		b.setSubscriberCounts(0, 0)
	})
}

// ReplayLast replays the most recent events into the provided channel in order.
func (b *Bus[T]) ReplayLast(count int, subscriber chan<- T) {
	if b == nil || subscriber == nil {
		return
	}
	events := b.historySnapshot(count)
	for _, event := range events {
		subscriber <- event
	}
}

// DumpHistory returns a copy of the stored event history in order.
func (b *Bus[T]) DumpHistory() []T {
	return b.historySnapshot(0)
}

type subscription[T any] struct {
	id     uint64
	ch     chan T
	filter func(T) bool
}

func (b *Bus[T]) sendToSubscriber(sub subscription[T], event T, eventType string) {
	if b.options.BlockOnFull {
		b.blockingSend(sub, event, eventType)
		return
	}
	b.nonBlockingSend(sub, event, eventType)
}

func (b *Bus[T]) nonBlockingSend(sub subscription[T], event T, eventType string) {
	delivered := b.safeSend(sub, func() bool {
		select {
		case sub.ch <- event:
			return true
		default:
			return false
		}
	})
	if !delivered {
		b.incDropped(eventType)
	}
}

func (b *Bus[T]) blockingSend(sub subscription[T], event T, eventType string) {
	start := time.Now()
	delivered := b.safeSend(sub, func() bool {
		if b.options.WriteTimeout <= 0 {
			sub.ch <- event
			return true
		}
		timer := time.NewTimer(b.options.WriteTimeout)
		defer timer.Stop()
		select {
		case sub.ch <- event:
			return true
		case <-timer.C:
			return false
		}
	})
	elapsed := time.Since(start)

	if !delivered {
		b.incDropped(eventType)
		b.removeSubscriber(sub.id)
		if b.options.SlowSubscriberThreshold > 0 && elapsed >= b.options.SlowSubscriberThreshold {
			log.Printf("event bus %s: subscriber blocked for %s and timed out", b.busName(), elapsed)
		}
		return
	}

	if b.options.SlowSubscriberThreshold > 0 && elapsed >= b.options.SlowSubscriberThreshold {
		log.Printf("event bus %s: subscriber blocked for %s", b.busName(), elapsed)
	}
}

func (b *Bus[T]) safeSend(sub subscription[T], send func() bool) (delivered bool) {
	defer func() {
		if recover() != nil {
			b.removeSubscriber(sub.id)
			delivered = false
		}
	}()
	return send()
}

func (b *Bus[T]) removeSubscriber(id uint64) {
	if b == nil {
		return
	}
	var ch chan T
	var filtered int
	var unfiltered int
	removed := false
	b.mu.Lock()
	if existing, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		ch = existing.ch
		removed = true
	}
	if removed {
		filtered, unfiltered = b.countSubscribersLocked()
	}
	b.mu.Unlock()

	if removed && ch != nil {
		close(ch)
	}
	if removed {
		b.setSubscriberCounts(filtered, unfiltered)
	}
}

func (b *Bus[T]) filterAllows(sub subscription[T], event T) (allowed bool) {
	if sub.filter == nil {
		return true
	}
	defer func() {
		if recover() != nil {
			log.Printf("event bus %s: subscriber filter panicked", b.busName())
			b.removeSubscriber(sub.id)
			allowed = false
		}
	}()
	return sub.filter(event)
}

func (b *Bus[T]) countSubscribersLocked() (filtered int, unfiltered int) {
	for _, sub := range b.subscribers {
		if sub.filter == nil {
			unfiltered++
		} else {
			filtered++
		}
	}
	return filtered, unfiltered
}

func (b *Bus[T]) SubscriberCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}

func (b *Bus[T]) busName() string {
	if b.options.Name == "" {
		return "event_bus"
	}
	return b.options.Name
}

func (b *Bus[T]) eventType(event T) string {
	typed, ok := any(event).(typedEvent)
	if !ok {
		return "unknown"
	}
	value := typed.Type()
	if value == "" {
		return "unknown"
	}
	return value
}

func (b *Bus[T]) incPublished(eventType string) {
	if b == nil {
		return
	}
	b.published.Add(1)
	if b.registry == nil {
		return
	}
	b.registry.IncEventPublished(b.busName(), eventType)
}

func (b *Bus[T]) incDropped(eventType string) {
	if b == nil {
		return
	}
	b.dropped.Add(1)
	if b.registry == nil {
		b.maybeWarnDropRate()
		return
	}
	b.registry.IncEventDropped(b.busName(), eventType)
	b.maybeWarnDropRate()
}

func (b *Bus[T]) setSubscriberCounts(filtered, unfiltered int) {
	if b.registry == nil {
		return
	}
	b.registry.SetEventSubscriberCounts(b.busName(), filtered, unfiltered)
}

func (b *Bus[T]) appendHistoryLocked(event T) {
	if len(b.history) == 0 {
		return
	}
	b.history[b.historyNext] = event
	if b.historyCount < len(b.history) {
		b.historyCount++
	}
	b.historyNext = (b.historyNext + 1) % len(b.history)
}

func (b *Bus[T]) historySnapshot(count int) []T {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.history) == 0 || b.historyCount == 0 {
		return nil
	}
	total := b.historyCount
	if count <= 0 || count > total {
		count = total
	}
	start := 0
	if total == len(b.history) {
		start = (b.historyNext - count + len(b.history)) % len(b.history)
	} else {
		start = total - count
	}

	events := make([]T, 0, count)
	for i := 0; i < count; i++ {
		index := (start + i) % len(b.history)
		events = append(events, b.history[index])
	}
	return events
}

func (b *Bus[T]) maybeWarnDropRate() {
	if b == nil {
		return
	}
	threshold := b.options.DropWarningThreshold
	if threshold <= 0 {
		return
	}
	published := b.published.Load()
	if published == 0 {
		return
	}
	dropped := b.dropped.Load()
	if dropped == 0 {
		return
	}
	rate := float64(dropped) / float64(published)
	if rate < threshold {
		return
	}
	interval := b.options.DropWarningInterval
	if interval <= 0 {
		interval = defaultDropWarningInterval
	}
	now := time.Now()
	lastNanos := b.lastWarning.Load()
	if lastNanos > 0 {
		last := time.Unix(0, lastNanos)
		if now.Sub(last) < interval {
			return
		}
	}
	if !b.lastWarning.CompareAndSwap(lastNanos, now.UnixNano()) {
		return
	}
	log.Printf("event bus %s: drop rate %.2f%% (%d dropped of %d published)", b.busName(), rate*100, dropped, published)
}

var debugEventsEnabled = isEventDebugEnabled()

func isEventDebugEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("GESTALT_EVENT_DEBUG")))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isNil[T any](value T) bool {
	kind := reflect.ValueOf(value)
	if !kind.IsValid() {
		return true
	}
	switch kind.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		return kind.IsNil()
	default:
		return false
	}
}
