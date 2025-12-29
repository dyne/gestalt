package terminal

import "os/exec"

type Pty interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Resize(cols, rows uint16) error
}

type PtyFactory interface {
	Start(command string, args ...string) (Pty, *exec.Cmd, error)
}

type defaultPtyFactory struct{}

func (defaultPtyFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	return startPty(command, args...)
}

func DefaultPtyFactory() PtyFactory {
	return defaultPtyFactory{}
}
