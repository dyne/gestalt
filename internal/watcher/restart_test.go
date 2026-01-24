package watcher

import (
	"errors"
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

func TestScheduleRestartSetsTimer(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	watcher.scheduleRestart(errors.New("boom"))

	watcher.restartMutex.Lock()
	timer := watcher.restartTimer
	attempts := watcher.restartAttempts
	watcher.restartMutex.Unlock()

	if attempts != 1 {
		t.Fatalf("expected 1 restart attempt, got %d", attempts)
	}
	if timer == nil {
		t.Fatalf("expected restart timer to be set")
	}
	if timer != nil {
		timer.Stop()
		watcher.restartMutex.Lock()
		watcher.restartTimer = nil
		watcher.restartMutex.Unlock()
	}
}

func TestScheduleRestartSkipsWhenTimerActive(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	timer := time.NewTimer(time.Hour)
	defer timer.Stop()

	watcher.restartMutex.Lock()
	watcher.restartTimer = timer
	watcher.restartAttempts = 1
	watcher.restartMutex.Unlock()

	watcher.scheduleRestart(errors.New("boom"))

	watcher.restartMutex.Lock()
	attempts := watcher.restartAttempts
	watcher.restartMutex.Unlock()

	if attempts != 1 {
		t.Fatalf("expected restart attempts to remain 1, got %d", attempts)
	}
}

func TestPerformRestartResetsAttempts(t *testing.T) {
	watcher, err := New()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	watcher.mutex.Lock()
	watcher.closed = true
	watcher.mutex.Unlock()

	watcher.restartMutex.Lock()
	watcher.restartAttempts = 2
	watcher.restartMutex.Unlock()

	watcher.performRestart()

	watcher.restartMutex.Lock()
	attempts := watcher.restartAttempts
	watcher.restartMutex.Unlock()

	if attempts != 0 {
		t.Fatalf("expected restart attempts to reset, got %d", attempts)
	}
}
