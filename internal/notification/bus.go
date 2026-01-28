package notification

import (
	"context"

	"gestalt/internal/event"
)

var bus = event.NewBus[Event](context.Background(), event.BusOptions{
	Name: "notification_events",
})

func Bus() *event.Bus[Event] {
	return bus
}

func Publish(event Event) {
	if bus == nil {
		return
	}
	bus.Publish(event)
}

func PublishToast(level, message string) {
	if bus == nil {
		return
	}
	bus.Publish(NewToast(level, message))
}
