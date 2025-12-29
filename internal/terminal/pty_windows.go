//go:build windows

package terminal

import (
	"errors"
	"os/exec"
)

// Windows PTY support requires ConPTY (Windows 10+). The Unix implementation
// lives in pty_unix.go behind the !windows build tag. This stub keeps Windows
// builds compiling while PTY support is pending. Options are to implement
// ConPTY directly, use github.com/UserExistsError/conpty, or keep this stub.
var errConPTYUnavailable = errors.New("conpty support not implemented; Windows terminals are unavailable")

func startPty(command string, args ...string) (Pty, *exec.Cmd, error) {
	return nil, nil, errConPTYUnavailable
}
