package config

import "io"

type ConffileAction int

const (
	ConffileKeep ConffileAction = iota
	ConffileInstall
)

type ConffileDecision int

const (
	ConffileDecisionInstall ConffileDecision = iota
	ConffileDecisionKeep
	ConffileDecisionSkip
	ConffileDecisionPrompt
)

type ConffileDecisionInput struct {
	DestExists  bool
	HasBaseline bool
	LocalHash   string
	OldHash     string
	NewHash     string
}

func DecideConffile(input ConffileDecisionInput) ConffileDecision {
	if !input.DestExists {
		return ConffileDecisionInstall
	}
	if !input.HasBaseline {
		return ConffileDecisionPrompt
	}
	if input.LocalHash == input.NewHash {
		return ConffileDecisionSkip
	}
	if input.LocalHash == input.OldHash && input.NewHash != input.OldHash {
		return ConffileDecisionInstall
	}
	if input.LocalHash != input.OldHash && input.NewHash == input.OldHash {
		return ConffileDecisionKeep
	}
	if input.LocalHash != input.OldHash && input.NewHash != input.OldHash {
		return ConffileDecisionPrompt
	}
	return ConffileDecisionPrompt
}

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
