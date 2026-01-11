package config

import (
	"context"

	"gestalt/internal/event"
)

var bus = event.NewBus[event.ConfigEvent](context.Background(), event.BusOptions{
	Name: "config_events",
})

func Bus() *event.Bus[event.ConfigEvent] {
	return bus
}
