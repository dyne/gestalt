package flow

type EventSignal struct {
	EventID string            `json:"event_id"`
	Fields  map[string]string `json:"fields"`
}

func BuildEventSignal(fields map[string]string) EventSignal {
	return EventSignal{
		EventID: BuildEventID(fields),
		Fields:  fields,
	}
}
