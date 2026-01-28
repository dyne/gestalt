package flow

type ActivityRequest struct {
	EventID    string            `json:"event_id"`
	TriggerID  string            `json:"trigger_id"`
	ActivityID string            `json:"activity_id"`
	Event      map[string]string `json:"event"`
	Config     map[string]any    `json:"config"`
	OutputTail string            `json:"output_tail,omitempty"`
}
