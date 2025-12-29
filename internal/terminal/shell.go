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
	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return "", nil, errors.New("shell command is empty")
	}
	return fields[0], fields[1:], nil
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
