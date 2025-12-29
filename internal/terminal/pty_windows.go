//go:build windows

package terminal

import (
	"errors"
	"os/exec"
)

var errConPTYUnavailable = errors.New("conpty support not implemented")

func startPty(command string, args ...string) (Pty, *exec.Cmd, error) {
	return nil, nil, errConPTYUnavailable
}
