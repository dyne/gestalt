//go:build !windows

package terminal

import (
	"bufio"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTerminateProcessTreeKillsGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & child=$!; echo $child; while true; do sleep 1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = terminateProcessTree(cmd, cmd.Process.Pid, processGroupID(cmd.Process.Pid), 100*time.Millisecond)
	}()

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	childText := strings.TrimSpace(line)
	childPID, err := strconv.Atoi(childText)
	if err != nil {
		t.Fatalf("parse child pid %q: %v", childText, err)
	}

	if err := terminateProcessTree(cmd, cmd.Process.Pid, processGroupID(cmd.Process.Pid), 200*time.Millisecond); err != nil {
		t.Fatalf("terminate: %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		if err := syscall.Kill(childPID, 0); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			t.Fatalf("check child process: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected child process %d to exit", childPID)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if cmd.ProcessState == nil {
		t.Fatalf("expected parent process to be reaped")
	}
}
