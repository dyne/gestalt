package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
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

func runCompleteAgents(args []string, out io.Writer, errOut io.Writer) int {
	cfg, err := parseCompletionArgs(args, errOut)
	if err != nil {
		return 1
	}
	names, err := fetchAgentNames(cfg)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 3
	}
	if len(names) == 0 {
		return 0
	}
	fmt.Fprint(out, strings.Join(names, " "))
	return 0
}

func parseCompletionArgs(args []string, errOut io.Writer) (Config, error) {
	fs := flag.NewFlagSet("gestalt-send", flag.ContinueOnError)
	fs.SetOutput(errOut)
	urlFlag := fs.String("url", "", "Gestalt server URL")
	tokenFlag := fs.String("token", "", "Gestalt auth token")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "usage: gestalt-send __complete-agents [--url URL] [--token TOKEN]")
	}
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	url := strings.TrimSpace(*urlFlag)
	if url == "" {
		url = strings.TrimSpace(os.Getenv("GESTALT_URL"))
	}
	if url == "" {
		url = defaultServerURL
	}

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GESTALT_TOKEN"))
	}

	return Config{
		URL:   url,
		Token: token,
	}, nil
}

const bashCompletionScript = `# Bash completion for gestalt-send
_gestalt_send_cached_agents() {
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/gestalt-send"
  local cache_file="$cache_dir/agents"
  local now
  now=$(date +%s)
  local cached=""

  if [[ -f "$cache_file" ]]; then
    local ts
    ts=$(head -n 1 "$cache_file" 2>/dev/null)
    if [[ "$ts" =~ ^[0-9]+$ ]]; then
      local age=$((now - ts))
      if (( age < 60 )); then
        cached=$(tail -n +2 "$cache_file" 2>/dev/null)
      fi
    fi
  fi

  if [[ -n "$cached" ]]; then
    echo "$cached"
    return
  fi

  local output
  output=$(gestalt-send __complete-agents 2>/dev/null)
  if [[ -n "$output" ]]; then
    mkdir -p "$cache_dir"
    {
      echo "$now"
      echo "$output"
    } > "$cache_file"
  fi

  echo "$output"
}

_gestalt_send_complete() {
  local cur prev words cword
  _init_completion || return

  if [[ "$cword" -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "completion --help --version --url --token --start --verbose --debug" -- "$cur") )
    return
  fi

  if [[ "$prev" == "completion" ]]; then
    COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") )
    return
  fi

  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "--help --version --url --token --start --verbose --debug" -- "$cur") )
    return
  fi

  if [[ "$cur" != -* ]]; then
    COMPREPLY=( $(compgen -W "$(_gestalt_send_cached_agents)" -- "$cur") )
    return
  fi
}

complete -F _gestalt_send_complete gestalt-send
`

const zshCompletionScript = `#compdef gestalt-send

_gestalt_send_cached_agents() {
  local cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/gestalt-send"
  local cache_file="$cache_dir/agents"
  local now=$(date +%s)
  local cached=""

  if [[ -f "$cache_file" ]]; then
    local ts=$(head -n 1 "$cache_file" 2>/dev/null)
    if [[ "$ts" =~ ^[0-9]+$ ]]; then
      local age=$((now - ts))
      if (( age < 60 )); then
        cached=$(tail -n +2 "$cache_file" 2>/dev/null)
      fi
    fi
  fi

  if [[ -n "$cached" ]]; then
    echo "$cached"
    return
  fi

  local output=$(gestalt-send __complete-agents 2>/dev/null)
  if [[ -n "$output" ]]; then
    mkdir -p "$cache_dir"
    {
      echo "$now"
      echo "$output"
    } > "$cache_file"
  fi
  echo "$output"
}

_gestalt_send() {
  local -a options
  options=(
    '--url[Gestalt server URL]:URL'
    '--token[Auth token]:TOKEN'
    '--start[Start agent if not running]'
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
      if [[ "$words[2]" == "completion" ]]; then
        _values 'shell' bash zsh
      else
        _values 'agent' ${=(_gestalt_send_cached_agents)}
      fi
      ;;
  esac
}

_gestalt_send "$@"
`
