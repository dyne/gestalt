# Gestalt

The Gestalt command manages an isolated Codex profile, agent skills, and the
Gestalt Mobile web interface. Requires Linux or macOS, Bash, Node.js 24 or newer,
and an installed Codex CLI for automatic Agents setup.

```sh
npm install -g @jaromil/gestalt
gestalt cli
```

When install scripts are allowed, global installation makes the command available
and automatically installs or reconciles Gestalt Agents, their context-mode runtime, and the isolated Codex
profile under `~/.codex-gestalt`. Your normal `~/.codex` profile remains separate.

npm 12 requires approval for install scripts. Allow this package's setup with:

```sh
npm install -g @jaromil/gestalt --allow-scripts=@jaromil/gestalt
```

Use the same flag for npm 12 updates unless the package is already approved in
your npm configuration. The package does not override blocked-script policies.

To add Mobile and the optional skill collection, run `gestalt install`.
Retry or refresh only Agents with `gestalt agents install` or `gestalt agents update`.
Setup errors fail the npm installation and print recovery instructions.

To install only the wrapper, use `npm install -g @jaromil/gestalt --ignore-scripts`;
run `gestalt agents install` later. Local dependency installs and npx inspection
do not automatically provision a user profile.

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
Installing a newer global npm version also reconciles Agents automatically.

You can also run the command without a global installation:

```sh
npx @jaromil/gestalt --help
```

[Documentation](https://dyne.github.io/gestalt/) ·
[Source and issues](https://github.com/dyne/gestalt)
