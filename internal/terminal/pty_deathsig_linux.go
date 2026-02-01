//go:build linux

package terminal

import "syscall"

func setPtyDeathSignal(attr *syscall.SysProcAttr) {
	if attr == nil {
		return
	}
	attr.Pdeathsig = syscall.SIGTERM
}
