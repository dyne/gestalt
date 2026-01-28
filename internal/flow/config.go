package flow

const ConfigVersion = 1

type EventTrigger struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	EventType string            `json:"event_type"`
	Where     map[string]string `json:"where"`
}

type ActivityBinding struct {
	ActivityID string         `json:"activity_id"`
	Config     map[string]any `json:"config"`
}

type Config struct {
	Version             int                         `json:"version"`
	Triggers            []EventTrigger              `json:"triggers"`
	BindingsByTriggerID map[string][]ActivityBinding `json:"bindings_by_trigger_id"`
}

func DefaultConfig() Config {
	return Config{
		Version:             ConfigVersion,
		Triggers:            []EventTrigger{},
		BindingsByTriggerID: map[string][]ActivityBinding{},
	}
}
