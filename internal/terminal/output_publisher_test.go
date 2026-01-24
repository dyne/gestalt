package terminal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestOutputPublisherDropOldest(t *testing.T) {
	publisher := &OutputPublisher{
		input: make(chan []byte, 1),
	}
	publisher.input <- []byte("first")

	publisher.publishDropOldest([]byte("second"))

	if dropped := atomic.LoadUint64(&publisher.dropped); dropped != 1 {
		t.Fatalf("expected 1 drop, got %d", dropped)
	}
	select {
	case got := <-publisher.input:
		if string(got) != "second" {
			t.Fatalf("expected latest chunk, got %q", string(got))
		}
	default:
		t.Fatalf("expected chunk to remain queued")
	}
}

func TestOutputPublisherDropNewest(t *testing.T) {
	publisher := &OutputPublisher{
		input: make(chan []byte, 1),
	}
	publisher.input <- []byte("first")

	publisher.publishDropNewest([]byte("second"))

	if dropped := atomic.LoadUint64(&publisher.dropped); dropped != 1 {
		t.Fatalf("expected 1 drop, got %d", dropped)
	}
	select {
	case got := <-publisher.input:
		if string(got) != "first" {
			t.Fatalf("expected original chunk, got %q", string(got))
		}
	default:
		t.Fatalf("expected chunk to remain queued")
	}
}

func TestOutputPublisherSample(t *testing.T) {
	publisher := &OutputPublisher{
		input:       make(chan []byte, 1),
		sampleEvery: 2,
	}

	publisher.publishSample([]byte("first"))
	publisher.publishSample([]byte("second"))
	publisher.publishSample([]byte("third"))

	if dropped := atomic.LoadUint64(&publisher.dropped); dropped != 2 {
		t.Fatalf("expected 2 drops, got %d", dropped)
	}
	select {
	case got := <-publisher.input:
		if string(got) != "second" {
			t.Fatalf("expected sampled chunk, got %q", string(got))
		}
	default:
		t.Fatalf("expected chunk to remain queued")
	}
}

func TestOutputPublisherPublishWithContextDoesNotBlock(t *testing.T) {
	publisher := &OutputPublisher{
		input:  make(chan []byte, 1),
		policy: OutputBackpressureBlock,
	}
	publisher.input <- []byte("first")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		publisher.PublishWithContext(ctx, []byte("second"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("publish should return promptly when context is canceled")
	}
}
