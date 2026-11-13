# Gestalt

The Gestalt command manages an isolated Codex profile, agent skills, and the
Gestalt Mobile web interface. Requires Linux or macOS, Bash, and Node.js 24 or newer.

```sh
npm install -g @jaromil/gestalt
gestalt install
gestalt doctor
```

Installing this package makes the `gestalt` command available. It has no install
hooks and does not download tools or change your Codex configuration until you
run a setup command. Your normal `~/.codex` profile remains separate.

```sh
gestalt cli
gestalt mobile
gestalt --help
```

Update the npm-installed command with:

```sh
npm install -g @jaromil/gestalt@latest
```

`gestalt update` updates the managed components and prints the npm command for
updating this package. It preserves the npm-managed executable and package metadata.

You can also run the command without a global installation:

```sh
npx @jaromil/gestalt --help
```

[Documentation](https://dyne.github.io/gestalt/) ·
[Source and issues](https://github.com/dyne/gestalt)
