# Get started with Gestalt

## Install with npm

You’ll need Node.js 24 or newer, npm, and the Codex CLI. Linux and macOS
also need Bash. See [installation options](./install.md) for platform support.

Run both commands from your npm project directory.

```sh
npm i @jaromil/gestalt
npx gestalt
```

`npx gestalt` prints the command list. If this is your first Gestalt setup,
install Agents and context-mode in the isolated Codex profile.

```sh
npx gestalt agents install
```

## Install extras

Run the commands for the extras you want. Serena and Playwright are available
on Linux and macOS.

```sh
npx gestalt skills install
npx gestalt serena install
npx gestalt playwright install
```

## Start a session

::: code-group

```sh [Terminal]
npx gestalt cli
```

```sh [Mobile web app]
npx gestalt mobile
```

:::

On Linux or macOS, Mobile installs on first launch and prints the URL to open
in your browser. The manager creates `~/gestalt` if needed and uses it as the
workspace root. Pass `--cwd` to use another directory. See
[your first session](./first-session.md) for what to do next.

## Update

Update the npm-installed manager from the same project directory.

```sh
npm i @jaromil/gestalt@latest
```

Update the components managed by Gestalt.

```sh
npx gestalt update
```
