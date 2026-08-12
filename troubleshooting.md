# Troubleshooting

## Start with the manager

```sh
gestalt doctor
```

It checks Node, npm, Codex, configured directories, installed plugins, and the
Mobile executable without printing prompts, model output, secrets, or arbitrary
environment values.

## `gestalt: command not found`

Add the default user binary directory to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Put the same line in your shell profile, then open a new terminal.

## Context mode is not prepared

If `ctx-doctor` or startup reports `CONTEXT_MODE_NOT_PREPARED`:

```sh
gestalt update
```

Confirm Node.js, `python3`, `make`, and a C/C++ compiler are installed. Also
check that a second context-mode marketplace variant is not enabled.

## Mobile will not start

```sh
codex --version
gestalt mobile --version
npm view gestalt-mobile version
```

Codex must be authenticated in the same user account. If Mobile reports an
incompatible Codex protocol, update Gestalt or install the Codex version
supported by that Mobile release.

## A Mobile option is rejected

```sh
gestalt mobile --help
```

The installed executable is authoritative. The manager forwards arguments
unchanged.

## Passkey origin errors

Check that `--public-origin` exactly matches the address in the browser,
including `https://` and any non-default port. Only `http://localhost` is valid
without HTTPS. Do not change the RP-ID hostname after devices are enrolled.

## Update fails halfway

Rerun `gestalt update`. Both the Agents runtime publication and the manager
binary installation are designed to replace prepared artifacts atomically.
Existing Codex and Mobile state is stored separately from install artifacts.
