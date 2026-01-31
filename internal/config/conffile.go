package config

import "io"

type ConffileAction int

const (
	ConffileKeep ConffileAction = iota
	ConffileInstall
)

type ConffileChoice struct {
	Action ConffileAction
}

type ConffilePrompt struct {
	RelPath  string
	DestPath string
	NewBytes []byte
}

type ConffileResolver struct {
	Interactive bool
	In          io.Reader
	Out         io.Writer
}

func (r *ConffileResolver) ResolveConflict(_ ConffilePrompt) (ConffileChoice, error) {
	if r == nil || !r.Interactive {
		return ConffileChoice{Action: ConffileKeep}, nil
	}
	return ConffileChoice{Action: ConffileKeep}, nil
}
