package terminal

import (
	"os/exec"
	"path/filepath"
)

type muxPtyFactory struct {
	tui   PtyFactory
	stdio PtyFactory
	debug bool
}

func NewMuxPtyFactory(tui PtyFactory, stdio PtyFactory, debug bool) PtyFactory {
	if tui == nil {
		tui = DefaultPtyFactory()
	}
	if stdio == nil {
		stdio = StdioPtyFactory()
	}
	return &muxPtyFactory{
		tui:   tui,
		stdio: stdio,
		debug: debug,
	}
}

func (f *muxPtyFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	if isCodexMCPCommand(command, args) {
		pty, cmd, err := f.stdio.Start(command, args...)
		if err != nil {
			return nil, nil, err
		}
		return newMCPPty(pty, f.debug), cmd, nil
	}
	return f.tui.Start(command, args...)
}

func isCodexMCPCommand(command string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	if filepath.Base(command) != "codex" {
		return false
	}
	return args[0] == "mcp-server"
}
