package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner executes tmux commands with optional stdin data.
type CommandRunner interface {
	Run(args []string, input []byte) ([]byte, error)
}

// Client executes tmux commands.
type Client struct {
	runner CommandRunner
}

// NewClient returns a tmux client using the default command runner.
func NewClient() *Client {
	return &Client{runner: execRunner{}}
}

// NewClientWithRunner returns a tmux client using a custom command runner.
func NewClientWithRunner(runner CommandRunner) *Client {
	return &Client{runner: runner}
}

// CreateSession creates a detached tmux session and optionally runs a command.
func (c *Client) CreateSession(name string, command []string) error {
	args := []string{"new-session", "-d", "-s", name}
	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}
	return c.run(args, nil)
}

// CreateWindow creates a new window in an existing session.
func (c *Client) CreateWindow(sessionName, windowName string, command []string) error {
	args := []string{"new-window"}
	if strings.TrimSpace(sessionName) != "" {
		args = append(args, "-t", sessionName)
	}
	if strings.TrimSpace(windowName) != "" {
		args = append(args, "-n", windowName)
	}
	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}
	return c.run(args, nil)
}

// CreatePane creates a new pane by splitting an existing target.
func (c *Client) CreatePane(target string, command []string) error {
	args := []string{"split-window", "-d", "-t", target}
	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}
	return c.run(args, nil)
}

// KillSession terminates a tmux session.
func (c *Client) KillSession(name string) error {
	return c.run([]string{"kill-session", "-t", name}, nil)
}

// KillWindow terminates a tmux window.
func (c *Client) KillWindow(target string) error {
	return c.run([]string{"kill-window", "-t", target}, nil)
}

// SendKeys sends keystrokes to a target pane.
func (c *Client) SendKeys(target string, keys ...string) error {
	args := append([]string{"send-keys", "-t", target}, keys...)
	return c.run(args, nil)
}

// LoadBuffer loads data into the tmux paste buffer.
func (c *Client) LoadBuffer(data []byte) error {
	return c.run([]string{"load-buffer", "-"}, data)
}

// PasteBuffer pastes the current buffer into a target pane.
func (c *Client) PasteBuffer(target string) error {
	return c.run([]string{"paste-buffer", "-t", target}, nil)
}

// PipePane pipes pane output to a command (typically a file append).
func (c *Client) PipePane(target, command string) error {
	return c.run([]string{"pipe-pane", "-t", target, "-o", command}, nil)
}

// CapturePane captures pane contents as raw text.
func (c *Client) CapturePane(target string) ([]byte, error) {
	output, err := c.runWithOutput([]string{"capture-pane", "-p", "-t", target}, nil)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// ResizePane resizes a pane to the requested dimensions.
func (c *Client) ResizePane(target string, cols, rows uint16) error {
	args := []string{"resize-pane", "-t", target}
	if cols > 0 {
		args = append(args, "-x", fmt.Sprintf("%d", cols))
	}
	if rows > 0 {
		args = append(args, "-y", fmt.Sprintf("%d", rows))
	}
	return c.run(args, nil)
}

// SelectWindow selects the target window.
func (c *Client) SelectWindow(target string) error {
	return c.run([]string{"select-window", "-t", target}, nil)
}

// HasSession reports whether the named session exists.
func (c *Client) HasSession(name string) (bool, error) {
	if c == nil || c.runner == nil {
		return false, errors.New("tmux runner unavailable")
	}
	output, err := c.runner.Run([]string{"has-session", "-t", name}, nil)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		if len(output) > 0 {
			return false, fmt.Errorf("tmux has-session failed: %s", bytes.TrimSpace(output))
		}
		return false, fmt.Errorf("tmux has-session failed: %w", err)
	}
	return true, nil
}

// HasWindow reports whether the named window exists inside a tmux session.
func (c *Client) HasWindow(sessionName, windowName string) (bool, error) {
	if c == nil || c.runner == nil {
		return false, errors.New("tmux runner unavailable")
	}
	args := []string{"list-windows", "-t", sessionName, "-F", "#{window_name}"}
	output, err := c.runner.Run(args, nil)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		if len(output) > 0 {
			return false, fmt.Errorf("tmux list-windows failed: %s", bytes.TrimSpace(output))
		}
		return false, fmt.Errorf("tmux list-windows failed: %w", err)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return false, nil
	}
	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		if strings.TrimRight(line, "\r") == windowName {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) run(args []string, input []byte) error {
	_, err := c.runWithOutput(args, input)
	return err
}

func (c *Client) runWithOutput(args []string, input []byte) ([]byte, error) {
	if c == nil || c.runner == nil {
		return nil, errors.New("tmux runner unavailable")
	}
	output, err := c.runner.Run(args, input)
	if err != nil {
		if len(output) > 0 {
			return nil, fmt.Errorf("tmux %s failed: %s", args[0], bytes.TrimSpace(output))
		}
		return nil, fmt.Errorf("tmux %s failed: %w", args[0], err)
	}
	return output, nil
}

type execRunner struct{}

func (execRunner) Run(args []string, input []byte) ([]byte, error) {
	cmd := exec.Command("tmux", args...)
	if len(input) > 0 {
		cmd.Stdin = bytes.NewReader(input)
	}
	return cmd.CombinedOutput()
}
