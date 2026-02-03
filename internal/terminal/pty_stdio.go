package terminal

import (
	"errors"
	"io"
	"os/exec"
)

type stdioPty struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p *stdioPty) Read(data []byte) (int, error) {
	return p.stdout.Read(data)
}

func (p *stdioPty) Write(data []byte) (int, error) {
	return p.stdin.Write(data)
}

func (p *stdioPty) Close() error {
	var errs []error
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, err)
		}
	}
	if p.stdout != nil {
		if err := p.stdout.Close(); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, err)
		}
	}
	if p.stderr != nil {
		if err := p.stderr.Close(); err != nil && !errors.Is(err, io.EOF) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *stdioPty) Resize(cols, rows uint16) error {
	return nil
}

type stdioPtyFactory struct{}

func (stdioPtyFactory) Start(command string, args ...string) (Pty, *exec.Cmd, error) {
	cmd := exec.Command(command, args...)
	setStdioProcAttr(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, nil, err
	}
	return &stdioPty{stdin: stdin, stdout: stdout, stderr: stderr}, cmd, nil
}

func StdioPtyFactory() PtyFactory {
	return stdioPtyFactory{}
}
