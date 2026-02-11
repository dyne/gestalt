package flow

import "testing"

func TestRenderTemplate(testingContext *testing.T) {
	request := ActivityRequest{
		EventID:    "event-1",
		TriggerID:  "trigger-1",
		ActivityID: "activity-1",
		OutputTail: "tail",
		Event: map[string]string{
			"foo":              "bar",
			"notify.plan_file": "PLAN.org",
		},
	}

	cases := map[string]string{
		"hello {{event.foo}}":                 "hello bar",
		"ids {{event_id}} {{trigger_id}}":     "ids event-1 trigger-1",
		"activity {{activity_id}}":            "activity activity-1",
		"tail {{output_tail}}":                "tail tail",
		"plan {{notify.plan_file}}":           "plan PLAN.org",
		"plan {{event.notify.plan_file}}":     "plan PLAN.org",
		"unknown {{missing}} ok":              "unknown  ok",
		"unfinished {{event.foo":              "unfinished {{event.foo",
		"adjacent {{event.foo}}{{event.foo}}": "adjacent barbar",
		"spaced {{  event.foo  }}":            "spaced bar",
		"empty {{}}":                          "empty ",
	}

	for input, expected := range cases {
		if got := RenderTemplate(input, request); got != expected {
			testingContext.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
	}
}
