package main

import "os"

func main() {
	cmd, cmdArgs := resolveCommand(os.Args[1:], defaultCommandDeps())
	os.Exit(cmd.Run(cmdArgs))
}
