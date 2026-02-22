package flow

import "testing"

func TestCanonicalNotifyEventType(testingContext *testing.T) {
	cases := map[string]string{
		"new-plan":            "plan-new",
		"New Plan":            "plan-new",
		"new__plan!!":         "plan-new",
		"plan-new":            "plan-new",
		"plan-L1-wip":         "plan-update",
		"plan-update":         "plan-update",
		"progress":            "plan-update",
		"start":               "work-start",
		"work-start":          "work-start",
		"work-progress":       "work-progress",
		"Finish":              "work-finish",
		"work-finish":         "work-finish",
		"agent-turn-complete": "agent-turn",
		"prompt-voice":        "prompt-voice",
		"prompt-text":         "prompt-text",
		"commit":              "git-commit",
		"git-commit":          "git-commit",
		"custom-type":         "agent-turn",
		"   ":                 "agent-turn",
		"!!!":                 "agent-turn",
	}

	for input, expected := range cases {
		if got := CanonicalNotifyEventType(input); got != expected {
			testingContext.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
	}
}
