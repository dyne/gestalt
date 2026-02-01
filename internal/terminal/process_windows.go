//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func processGroupID(pid int) int {
	return 0
}

func terminateProcessTree(cmd *exec.Cmd, pid, pgid int, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = pgid
	var errs []error
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		errs = append(errs, fmt.Errorf("kill process: %w", err))
	}
	if err := waitForProcessExit(cmd, timeout); err != nil && !errors.Is(err, os.ErrProcessDone) {
		errs = append(errs, fmt.Errorf("wait process: %w", err))
	}
	return errors.Join(errs...)
}

func waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil {
		return nil
	}
	if cmd.ProcessState != nil {
		return nil
	}
	if timeout <= 0 {
		return cmd.Wait()
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return nil
	}
}
