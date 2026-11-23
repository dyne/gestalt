# Gestalt

Gestalt manages an isolated Codex profile and its Agents, context-mode and
skills. It keeps the existing Linux and macOS routes, including optional Mobile.
The native core candidate was validated on Windows x64. Public npm version
`0.16.1` supports Linux and macOS. On Windows, check that the version you plan
to install lists `win32`, or use an explicitly supplied, verified tarball.

```sh
npm view @jaromil/gestalt@latest version os
```

Agents-only setup needs Node.js 24 or newer, npm and an installed, compatible
Codex CLI. Linux and macOS also require Bash. Native Windows x64 uses PowerShell
or cmd under a standard user account, without Bash, WSL, Developer Mode or a
compiler. On Windows, full `gestalt install` and the default `gestalt update`
also require Git on `PATH` for the Git-backed skill collection. Use Git to
clone or update a local Agents checkout.

Native Windows Serena, Playwright/Chrome and Mobile hosting are deferred. Core
Agents/context-mode setup and core updates report these omissions. Explicit
requests for deferred components fail before setup changes the profile.
Other Windows architectures and UNC paths were outside the validation.
macOS retains its best-effort Unix behavior.

With npm 11, install a release available for your platform.

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts
gestalt cli
```

When install scripts are allowed, global installation makes the command available
and automatically installs or reconciles Gestalt Agents, their context-mode runtime, and the isolated Codex
profile under `~/.codex-gestalt`. Your normal `~/.codex` profile remains separate.

npm 12 requires approval for install scripts. Allow the registry package's
lifecycle script with the following command. npm 12.2.2 requires at least
Node 24.15.0 when using Node 24.

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts --allow-scripts=@jaromil/gestalt
```

Use the same flag for npm 12 updates unless the package is already approved in
your npm configuration. For a local tarball, approve its resolved `file:` identity.
This PowerShell example uses a tarball supplied separately.

```powershell
$tarball = (Resolve-Path .\gestalt-candidate.tgz).Path
npm install -g "--allow-scripts=file:$tarball" $tarball
```

On Unix, `gestalt install` can also add Mobile and the optional skill collection.
On Windows it installs the available core and reports deferred components.
Retry or refresh only Agents with `gestalt agents install` or `gestalt agents update`.
Setup errors fail the npm installation and print recovery instructions.

To install only the wrapper, use `npm install -g @jaromil/gestalt@latest --ignore-scripts`;
run `gestalt agents install` later. Local dependency installs and npx inspection
do not automatically provision a user profile.

Run `gestalt --help`, `gestalt --version` and `gestalt doctor` to inspect the
installed command. On Windows use `gestalt.cmd` explicitly from PowerShell
or cmd if needed. The version's first line names the manager version. The
remaining lines report local component status, including deferrals.

## Skill profiles

The npm CLI shows a skill-profile picker before an interactive `gestalt cli`
session. Choose a saved profile, Manage skill profiles, or None. Escape
cancels the launch. Open the editor directly with `gestalt skill-profiles`.

The editor uses the current workspace and saves profiles alongside Gestalt
Mobile's profiles in `~/.gestalt/skill-profiles`, or in `GESTALT_HOME` when set.
None starts the session with only mandatory Gestalt skills. A saved profile
changes skills for this launch. Browser consent is stored with the
profile and the platform's existing availability rules still apply.

Use `gestalt cli --skill-profile NAME` to select a saved profile directly or
`gestalt cli --no-skill-profile` to use the native Codex selection. Piped and
administrative commands run without the picker.

Update the npm-installed command with:

```sh
npm install -g @jaromil/gestalt@latest --foreground-scripts
```

For an existing local dependency, run `npm install @jaromil/gestalt@latest --foreground-scripts`
from its project. The explicit tag selects the current release even when the project's
saved version range excludes it. Foreground scripts show the installed version report
when npm runs the install hook. If npm reports that everything is already up to date,
use `npm exec -- gestalt version` in that project to see the report again.

`gestalt update` updates the managed components and prints the npm command for
updating this package. It preserves the npm-managed executable and package metadata.
Installing a newer global npm version also reconciles Agents automatically.

To restore a previously validated compatible npm version, install it by its
exact version with `--ignore-scripts`. This restores the command and package
files. Retain the current profile and data, then check component compatibility
before reconciling Agents again. An unknown cleanup state after a failed setup
requires inspection of the retained state before retrying.

You can also run the command without a global installation:

```sh
npx @jaromil/gestalt --help
```

[Documentation](https://dyne.github.io/gestalt/) ·
[Source and issues](https://github.com/dyne/gestalt)
