package terminal

import (
	"os/exec"
)

type muxPtyFactory struct {
	tui PtyFactory
}

func NewMuxPtyFactory(tui PtyFactory, stdio PtyFactory, debug bool) PtyFactory {
	_ = stdio
	_ = debug
	if tui == nil {
		tui = DefaultPtyFactory()
	}
	return &muxPtyFactory{
		tui: tui,
	}
}

func (f *muxPtyFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	return f.tui.Start(command, args...)
}
