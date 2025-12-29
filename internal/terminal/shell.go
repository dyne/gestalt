package terminal

import (
	"os"
	"runtime"
)

func DefaultShell() string {
	return defaultShellFor(runtime.GOOS, os.Getenv)
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
