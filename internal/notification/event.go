package notification

import "time"

const EventTypeToast = "toast"

type Event struct {
	EventType  string    `json:"type"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	OccurredAt time.Time `json:"timestamp"`
}

func NewToast(level, message string) Event {
	return Event{
		EventType:  EventTypeToast,
		Level:      level,
		Message:    message,
		OccurredAt: time.Now().UTC(),
	}
}

func (e Event) Type() string {
	return e.EventType
}

func (e Event) Timestamp() time.Time {
	return e.OccurredAt
}
