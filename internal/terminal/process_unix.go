//go:build !windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func processGroupID(pid int) int {
	if pid <= 0 {
		return 0
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return 0
	}
	return pgid
}

func terminateProcessTree(cmd *exec.Cmd, pid, pgid int, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if pid <= 0 {
		pid = cmd.Process.Pid
	}
	if pgid <= 0 {
		pgid = processGroupID(pid)
	}

	var errs []error
	if err := signalProcessGroup(pid, pgid, syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		errs = append(errs, fmt.Errorf("signal group: %w", err))
	}

	exited, waitErr := waitForProcessExit(cmd, timeout)
	if waitErr != nil && !errors.Is(waitErr, os.ErrProcessDone) && !isExpectedProcessExit(waitErr) {
		errs = append(errs, fmt.Errorf("wait process: %w", waitErr))
	}

	if !exited {
		if err := signalProcessGroup(pid, pgid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("kill group: %w", err))
		}
		if err := waitForProcess(cmd); err != nil && !errors.Is(err, os.ErrProcessDone) && !isExpectedProcessExit(err) {
			errs = append(errs, fmt.Errorf("wait process: %w", err))
		}
	}

	return errors.Join(errs...)
}

func signalProcessGroup(pid, pgid int, sig syscall.Signal) error {
	if pgid > 0 {
		return syscall.Kill(-pgid, sig)
	}
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(pid, sig)
}

func waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) (bool, error) {
	if cmd == nil {
		return true, nil
	}
	if cmd.ProcessState != nil {
		return true, nil
	}
	if timeout <= 0 {
		err := waitForProcess(cmd)
		return true, err
	}

	done := make(chan error, 1)
	go func() {
		done <- waitForProcess(cmd)
	}()

	select {
	case err := <-done:
		return true, err
	case <-time.After(timeout):
		return false, nil
	}
}

func waitForProcess(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	if cmd.ProcessState != nil {
		return nil
	}
	return cmd.Wait()
}

func isExpectedProcessExit(err error) bool {
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
