//go:build !windows

package terminal

import (
	"os/exec"
	"syscall"
)

func setStdioProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	setPtyDeathSignal(cmd.SysProcAttr)
}
