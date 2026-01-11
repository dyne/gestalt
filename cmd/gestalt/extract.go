package main

import (
	"fmt"
	"os"
)

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func runExtractConfig() int {
	fmt.Fprintln(os.Stdout, "Config extraction runs automatically at startup into .gestalt/config; --extract-config is now a no-op.")
	return 0
}
