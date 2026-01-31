package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

type DiffRunner func(oldPath, newPath string) (string, error)

type ConffileResolver struct {
	Interactive bool
	In          io.Reader
	Out         io.Writer
	DiffRunner  DiffRunner

	applyAllInstall bool
}

func (r *ConffileResolver) ResolveConflict(prompt ConffilePrompt) (ConffileChoice, error) {
	if r == nil || !r.Interactive {
		return ConffileChoice{Action: ConffileKeep}, nil
	}
	if r.applyAllInstall {
		return ConffileChoice{Action: ConffileInstall}, nil
	}
	if r.In == nil || r.Out == nil {
		return ConffileChoice{Action: ConffileKeep}, nil
	}

	reader := bufio.NewReader(r.In)
	for {
		if err := r.writePrompt(prompt); err != nil {
			return ConffileChoice{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return ConffileChoice{}, err
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if choice == "" {
			return ConffileChoice{Action: ConffileKeep}, nil
		}
		switch choice {
		case "y", "i":
			return ConffileChoice{Action: ConffileInstall}, nil
		case "n", "o":
			return ConffileChoice{Action: ConffileKeep}, nil
		case "a":
			r.applyAllInstall = true
			return ConffileChoice{Action: ConffileInstall}, nil
		case "d":
			r.showDiff(prompt)
			continue
		default:
			if err := r.writeInvalidChoice(); err != nil {
				return ConffileChoice{}, err
			}
			if errors.Is(err, io.EOF) {
				return ConffileChoice{Action: ConffileKeep}, nil
			}
		}
	}
}

func (r *ConffileResolver) writePrompt(prompt ConffilePrompt) error {
	_, err := fmt.Fprintf(r.Out, "Configuration file 'config/%s'\n", prompt.RelPath)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, " ==> File on system has been modified."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, " ==> Package distributor has shipped an updated version."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, "   What would you like to do about it?  Your options are:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, "    Y or I  : install the package maintainer's version"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, "    N or O  : keep your currently-installed version"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, "      D     : show the differences between the versions"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, "      A     : replace this and all other configs"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.Out, " The default action is to keep your current version."); err != nil {
		return err
	}
	_, err = fmt.Fprint(r.Out, "What do you want to do? [N] ")
	return err
}

func (r *ConffileResolver) writeInvalidChoice() error {
	_, err := fmt.Fprintln(r.Out, "Please enter Y, N, D, or A.")
	return err
}

func (r *ConffileResolver) showDiff(prompt ConffilePrompt) {
	if r.DiffRunner == nil || len(prompt.NewBytes) == 0 {
		_, _ = fmt.Fprintln(r.Out, "files differ")
		return
	}
	dir := filepath.Dir(prompt.DestPath)
	tempFile, err := os.CreateTemp(dir, ".gestalt-config-")
	if err != nil {
		_, _ = fmt.Fprintln(r.Out, "files differ")
		return
	}
	name := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(name)
	}()
	if _, err := tempFile.Write(prompt.NewBytes); err != nil {
		_, _ = fmt.Fprintln(r.Out, "files differ")
		return
	}
	if err := tempFile.Close(); err != nil {
		_, _ = fmt.Fprintln(r.Out, "files differ")
		return
	}
	output, err := r.DiffRunner(prompt.DestPath, name)
	if err != nil || strings.TrimSpace(output) == "" {
		_, _ = fmt.Fprintln(r.Out, "files differ")
		return
	}
	_, _ = fmt.Fprint(r.Out, output)
}
