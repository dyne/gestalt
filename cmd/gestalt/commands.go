package main

import (
	"io"
	"os"
)

type command interface {
	Run(args []string) int
}

type commandDeps struct {
	Stdout            io.Writer
	Stderr            io.Writer
	RunServer         func(args []string) int
	RunValidateSkill  func(args []string) int
	RunValidateConfig func(args []string) int
	RunCompletion     func(args []string, out io.Writer, errOut io.Writer) int
	RunExtractConfig  func() int
}

func defaultCommandDeps() commandDeps {
	return commandDeps{
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		RunServer:         runServer,
		RunValidateSkill:  runValidateSkill,
		RunValidateConfig: runValidateConfig,
		RunCompletion:     runCompletion,
		RunExtractConfig:  runExtractConfig,
	}
}

type serverCommand struct {
	deps commandDeps
}

func (c serverCommand) Run(args []string) int {
	return c.deps.RunServer(args)
}

type validateSkillCommand struct {
	deps commandDeps
}

func (c validateSkillCommand) Run(args []string) int {
	return c.deps.RunValidateSkill(args)
}

type validateConfigCommand struct {
	deps commandDeps
}

func (c validateConfigCommand) Run(args []string) int {
	return c.deps.RunValidateConfig(args)
}

type completionCommand struct {
	deps commandDeps
}

func (c completionCommand) Run(args []string) int {
	return c.deps.RunCompletion(args, c.deps.Stdout, c.deps.Stderr)
}

type extractConfigCommand struct {
	deps commandDeps
}

func (c extractConfigCommand) Run(args []string) int {
	return c.deps.RunExtractConfig()
}

func resolveCommand(args []string, deps commandDeps) (command, []string) {
	if len(args) > 0 && args[0] == "validate-skill" {
		return validateSkillCommand{deps: deps}, args[1:]
	}
	if len(args) > 1 && args[0] == "config" && args[1] == "validate" {
		return validateConfigCommand{deps: deps}, args[2:]
	}
	if len(args) > 0 && args[0] == "completion" {
		return completionCommand{deps: deps}, args[1:]
	}
	if hasFlag(args, "--extract-config") {
		return extractConfigCommand{deps: deps}, args
	}
	return serverCommand{deps: deps}, args
}
