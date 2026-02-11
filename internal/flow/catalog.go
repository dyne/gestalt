package flow

type ActivityField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

type ActivityDef struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Fields      []ActivityField `json:"fields"`
}

var activityCatalog = []ActivityDef{
	{
		ID:          "toast_notification",
		Label:       "Toast notification",
		Description: "Show a toast in connected UIs when a trigger matches.",
		Fields: []ActivityField{
			{Key: "level", Label: "Level", Type: "string", Required: true},
			{Key: "message_template", Label: "Message template", Type: "string", Required: true},
		},
	},
	{
		ID:          "post_webhook",
		Label:       "Post webhook",
		Description: "POST the event payload to an external HTTP endpoint.",
		Fields: []ActivityField{
			{Key: "url", Label: "URL", Type: "string", Required: true},
			{Key: "headers_json", Label: "Headers JSON", Type: "string"},
			{Key: "body_template", Label: "Body template", Type: "string"},
		},
	},
	{
		ID:          "send_to_terminal",
		Label:       "Send message to session",
		Description: "Send a formatted message to a specific session (preferred) or agent fallback.",
		Fields: []ActivityField{
			{Key: "target_session_id", Label: "Target session id (preferred)", Type: "string"},
			{Key: "target_agent_name", Label: "Target agent (fallback)", Type: "string"},
			{Key: "include_terminal_output", Label: "Include terminal output", Type: "bool"},
			{Key: "output_tail_lines", Label: "Output tail lines", Type: "int"},
			{Key: "message_template", Label: "Message template", Type: "string", Required: true},
		},
	},
	{
		ID:          "spawn_agent_session",
		Label:       "Spawn agent session",
		Description: "Create (or reuse) an agent session and optionally send a message.",
		Fields: []ActivityField{
			{Key: "agent_id", Label: "Agent id", Type: "string", Required: true},
			{Key: "reuse_if_running", Label: "Reuse if running", Type: "bool"},
			{Key: "use_workflow", Label: "Use workflow", Type: "bool"},
			{Key: "title_template", Label: "Title template", Type: "string"},
			{Key: "message_template", Label: "Message template", Type: "string"},
		},
	},
}

func ActivityCatalog() []ActivityDef {
	return cloneActivityDefs(activityCatalog)
}

func cloneActivityDefs(source []ActivityDef) []ActivityDef {
	if len(source) == 0 {
		return []ActivityDef{}
	}
	cloned := make([]ActivityDef, 0, len(source))
	for _, def := range source {
		fields := make([]ActivityField, len(def.Fields))
		copy(fields, def.Fields)
		cloned = append(cloned, ActivityDef{
			ID:          def.ID,
			Label:       def.Label,
			Description: def.Description,
			Fields:      fields,
		})
	}
	return cloned
}
