package watcher

import (
	"testing"
	"time"
)

func TestRestartDelayBackoff(t *testing.T) {
	cases := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 0, expected: restartBaseDelay},
		{attempt: 1, expected: restartBaseDelay * 2},
		{attempt: 2, expected: restartBaseDelay * 4},
	}

	for _, testCase := range cases {
		if got := restartDelay(testCase.attempt); got != testCase.expected {
			t.Fatalf("attempt %d: expected %s, got %s", testCase.attempt, testCase.expected, got)
		}
	}
}
