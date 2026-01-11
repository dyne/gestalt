package event

import (
	"context"
	"testing"
)

func BenchmarkBusPublishNoSubscribers(b *testing.B) {
	bus := NewBus[int](context.Background(), BusOptions{})
	b.Cleanup(bus.Close)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(i)
	}
}

func BenchmarkBusPublishWithSubscribers(b *testing.B) {
	bus := NewBus[int](context.Background(), BusOptions{
		SubscriberBufferSize: 1,
	})
	b.Cleanup(bus.Close)

	cancels := make([]func(), 0, 100)
	for i := 0; i < 100; i++ {
		_, cancel := bus.Subscribe()
		cancels = append(cancels, cancel)
	}
	b.Cleanup(func() {
		for _, cancel := range cancels {
			cancel()
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(i)
	}
}

func BenchmarkBusSubscribeUnsubscribe(b *testing.B) {
	bus := NewBus[int](context.Background(), BusOptions{})
	b.Cleanup(bus.Close)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, cancel := bus.Subscribe()
		cancel()
	}
}
