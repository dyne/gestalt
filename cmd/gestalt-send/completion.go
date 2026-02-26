package main

import (
	"fmt"
	"io"
)

func runCompletion(args []string, out io.Writer, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: gestalt-send completion [bash|zsh]")
		return 1
	}
	switch args[0] {
	case "bash":
		_, _ = io.WriteString(out, bashCompletionScript)
		return 0
	case "zsh":
		_, _ = io.WriteString(out, zshCompletionScript)
		return 0
	default:
		fmt.Fprintln(errOut, "usage: gestalt-send completion [bash|zsh]")
		return 1
	}
}

const bashCompletionScript = `# Bash completion for gestalt-send
_gestalt_send_complete() {
  local cur prev words cword
  _init_completion || return

  if [[ "$cword" -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "completion --help --version --host --port --token --from --verbose --debug" -- "$cur") )
    return
  fi

  if [[ "$prev" == "completion" ]]; then
    COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") )
    return
  fi

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "--help --version --host --port --token --from --verbose --debug" -- "$cur") )
    return
  fi
}

complete -F _gestalt_send_complete gestalt-send
`

const zshCompletionScript = `#compdef gestalt-send

_gestalt_send() {
  local -a options
  options=(
    '--host[Gestalt server host]:HOST'
    '--port[Gestalt server port]:PORT'
    '--token[Auth token]:TOKEN'
    '--from[Calling session reference for log attribution]:SESSION'
    '--verbose[Verbose output]'
    '--debug[Debug output]'
    '--help[Show help]'
    '--version[Print version]'
  )

  _arguments -C \
    '1: :->subcmd' \
    '*::arg:->args'

  case $state in
    subcmd)
      _values 'subcommand' completion
      ;;
    args)
      _values 'shell' bash zsh
      ;;
  esac
}

_gestalt_send "$@"
`
