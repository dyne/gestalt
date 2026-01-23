package terminal

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

func DefaultShell() string {
	return defaultShellFor(runtime.GOOS, os.Getenv)
}

func splitCommandLine(commandLine string) (string, []string, error) {
	fields, err := parseCommandLine(commandLine)
	if err != nil {
		return "", nil, err
	}
	if len(fields) == 0 {
		return "", nil, errors.New("shell command is empty")
	}
	return fields[0], fields[1:], nil
}

func parseCommandLine(commandLine string) ([]string, error) {
	const (
		stateNone = iota
		stateSingle
		stateDouble
	)
	state := stateNone
	escaped := false
	tokenStarted := false
	fields := []string{}
	var current strings.Builder

	flushToken := func() {
		if !tokenStarted {
			return
		}
		fields = append(fields, current.String())
		current.Reset()
		tokenStarted = false
	}

	for i := 0; i < len(commandLine); i++ {
		ch := commandLine[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		switch state {
		case stateNone:
			if isShellSpace(ch) {
				flushToken()
				continue
			}
			tokenStarted = true
			switch ch {
			case '\\':
				escaped = true
			case '\'':
				state = stateSingle
			case '"':
				state = stateDouble
			default:
				current.WriteByte(ch)
			}
		case stateSingle:
			tokenStarted = true
			if ch == '\'' {
				state = stateNone
				continue
			}
			current.WriteByte(ch)
		case stateDouble:
			tokenStarted = true
			switch ch {
			case '"':
				state = stateNone
			case '\\':
				escaped = true
			default:
				current.WriteByte(ch)
			}
		}
	}

	if escaped {
		return nil, errors.New("unterminated escape in shell command")
	}
	if state == stateSingle || state == stateDouble {
		return nil, errors.New("unterminated quote in shell command")
	}
	flushToken()
	return fields, nil
}

func isShellSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func defaultShellFor(goos string, getenv func(string) string) string {
	if goos == "windows" {
		if shell := getenv("ComSpec"); shell != "" {
			return shell
		}
		if shell := getenv("COMSPEC"); shell != "" {
			return shell
		}
		return "cmd.exe"
	}

	if shell := getenv("SHELL"); shell != "" {
		return shell
	}

	return "/bin/bash"
}
