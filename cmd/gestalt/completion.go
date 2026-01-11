package main

import (
	"fmt"
	"io"
)

func runCompletion(args []string, out io.Writer, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: gestalt completion [bash|zsh]")
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
		fmt.Fprintln(errOut, "usage: gestalt completion [bash|zsh]")
		return 1
	}
}

const bashCompletionScript = `# Bash completion for gestalt
_gestalt_complete() {
  local cur prev
  _get_comp_words_by_ref -n : cur prev

  if [[ "$prev" == "completion" ]]; then
    COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") )
    return
  fi

  if [[ "$prev" == "validate-skill" ]]; then
    COMPREPLY=( $(compgen -f -- "$cur") )
    return
  fi

  if [[ "$prev" == "--shell" ]]; then
    COMPREPLY=( $(compgen -W "/bin/bash /bin/zsh /bin/sh" -- "$cur") )
    return
  fi

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "--port --backend-port --shell --token --session-persist --session-dir --session-buffer-lines --session-retention-days --input-history-persist --input-history-dir --max-watches --verbose --quiet --force-upgrade --help --version --extract-config" -- "$cur") )
    return
  fi

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "validate-skill completion" -- "$cur") )
  fi
}

complete -F _gestalt_complete gestalt
`

const zshCompletionScript = `#compdef gestalt
_gestalt_complete() {
  local -a flags
  flags=(
    '--port[HTTP frontend port]'
    '--backend-port[Backend API port]'
    '--shell[Default shell command]'
    '--token[Auth token for REST/WS]'
    '--session-persist[Persist terminal sessions to disk]'
    '--session-dir[Session log directory]'
    '--session-buffer-lines[Session buffer lines]'
    '--session-retention-days[Session retention days]'
    '--input-history-persist[Persist input history]'
    '--input-history-dir[Input history directory]'
    '--max-watches[Max active watches]'
    '--verbose[Enable verbose logging]'
    '--quiet[Reduce logging to warnings]'
    '--force-upgrade[Bypass config version compatibility checks]'
    '--help[Show help]'
    '--version[Print version and exit]'
    '--extract-config[Extract embedded defaults]'
  )

  case ${words[2]} in
    completion)
      _values 'shells' bash zsh
      return
      ;;
    validate-skill)
      _files
      return
      ;;
  esac

  _arguments -s $flags '1:subcommand:(validate-skill completion)' '*::arg:->args'
}

_gestalt_complete "$@"
`
