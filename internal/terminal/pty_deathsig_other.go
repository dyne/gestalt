//go:build !linux && !windows

package terminal

import "syscall"

func setPtyDeathSignal(attr *syscall.SysProcAttr) {
	_ = attr
}
