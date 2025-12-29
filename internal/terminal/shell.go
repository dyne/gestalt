package terminal

import (
	"os"
	"runtime"
)

func DefaultShell() string {
	if runtime.GOOS == "windows" {
		if shell := os.Getenv("ComSpec"); shell != "" {
			return shell
		}
		if shell := os.Getenv("COMSPEC"); shell != "" {
			return shell
		}
		return "cmd.exe"
	}

	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	return "/bin/bash"
}
