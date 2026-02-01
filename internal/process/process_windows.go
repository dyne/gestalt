//go:build windows

package process

import (
	"context"
	"os"
	"time"
)

func GroupID(pid int) int {
	return 0
}

func stopProcess(ctx context.Context, pid, pgid int, wait func(context.Context) error) error {
	if pid <= 0 {
		return nil
	}
	_ = pgid
	process, err := os.FindProcess(pid)
	if err != nil {
		return ErrProcessNotFound
	}
	_ = process.Kill()
	return waitForExit(ctx, pid, wait)
}

func waitForExit(ctx context.Context, pid int, wait func(context.Context) error) error {
	if wait != nil {
		return wait(ctx)
	}
	if pid <= 0 {
		return nil
	}
	timeout := defaultStopTimeout
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ctx.Err()
			}
			if remaining < timeout {
				timeout = remaining
			}
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		process, err := os.FindProcess(pid)
		if err != nil || process == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
