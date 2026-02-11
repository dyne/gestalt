package flow

import "testing"

func TestCanonicalNotifyEventType(testingContext *testing.T) {
	cases := map[string]string{
		"new-plan":    "notify_new_plan",
		"New Plan":    "notify_new_plan",
		"new__plan!!": "notify_new_plan",
		"progress":    "notify_progress",
		"Finish":      "notify_finish",
		"custom-type": "notify_event",
		"   ":         "notify_event",
		"!!!":         "notify_event",
	}

	for input, expected := range cases {
		if got := CanonicalNotifyEventType(input); got != expected {
			testingContext.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
	}
}
