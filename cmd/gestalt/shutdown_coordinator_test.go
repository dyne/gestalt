package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
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
