package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestShutdownCoordinatorRunsInOrder(t *testing.T) {
	coordinator := newShutdownCoordinator(nil)
	order := []string{}

	coordinator.Add("first", func(context.Context) error {
		order = append(order, "first")
		return nil
	})
	coordinator.Add("second", func(context.Context) error {
		order = append(order, "second")
		return errors.New("fail")
	})
	coordinator.Add("third", func(context.Context) error {
		order = append(order, "third")
		return nil
	})

	err := coordinator.Run(context.Background())
	if err == nil {
		t.Fatalf("expected shutdown error")
	}

	expected := []string{"first", "second", "third"}
	if !reflect.DeepEqual(order, expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
}

func TestShutdownCoordinatorPhaseTimeout(t *testing.T) {
	coordinator := newShutdownCoordinator(nil)
	coordinator.phaseTimeout = 10 * time.Millisecond
	coordinator.totalTimeout = 100 * time.Millisecond

	order := []string{}
	coordinator.Add("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	coordinator.Add("fast", func(ctx context.Context) error {
		order = append(order, "fast")
		return nil
	})

	err := coordinator.Run(context.Background())
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	expected := []string{"fast"}
	if !reflect.DeepEqual(order, expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
}
