//go:build !windows

package process

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func GroupID(pid int) int {
	if pid <= 0 {
		return 0
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return 0
	}
	return pgid
}

func stopProcess(ctx context.Context, pid, pgid int, wait func(context.Context) error) error {
	if pid <= 0 {
		return nil
	}
	if !isProcessAlive(pid) {
		return ErrProcessNotFound
	}
	termErr := signalProcessGroup(pid, pgid, syscall.SIGTERM)
	if errors.Is(termErr, syscall.ESRCH) {
		termErr = nil
	}
	waitErr := waitForExit(ctx, pid, wait)
	if isExpectedExit(waitErr) {
		waitErr = nil
	}
	if waitErr == nil {
		return termErr
	}
	killErr := signalProcessGroup(pid, pgid, syscall.SIGKILL)
	if errors.Is(killErr, syscall.ESRCH) {
		killErr = nil
	}
	_ = waitForExit(ctx, pid, wait)
	return errors.Join(termErr, waitErr, killErr)
}

func signalProcessGroup(pid, pgid int, sig syscall.Signal) error {
	target := pid
	if pgid > 0 {
		target = -pgid
	}
	return syscall.Kill(target, sig)
}

func waitForExit(ctx context.Context, pid int, wait func(context.Context) error) error {
	if wait != nil {
		return wait(ctx)
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
		if !isProcessAlive(pid) {
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

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

func isExpectedExit(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}
	return status.Signaled()
}
