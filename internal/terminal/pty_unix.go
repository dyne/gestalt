//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type filePty struct {
	file *os.File
}

func (p *filePty) Read(data []byte) (int, error) {
	return p.file.Read(data)
}

func (p *filePty) Write(data []byte) (int, error) {
	return p.file.Write(data)
}

func (p *filePty) Close() error {
	return p.file.Close()
}

func (p *filePty) Resize(cols, rows uint16) error {
	return pty.Setsize(p.file, &pty.Winsize{Cols: cols, Rows: rows})
}

func startPty(command string, args ...string) (Pty, *exec.Cmd, error) {
	cmd := exec.Command(command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	setPtyDeathSignal(cmd.SysProcAttr)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, err
	}

	return &filePty{file: ptmx}, cmd, nil
}
