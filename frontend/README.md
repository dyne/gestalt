# Svelte + Vite

This template should help get you started developing with Svelte in Vite.

## Recommended IDE Setup

[VS Code](https://code.visualstudio.com/) + [Svelte](https://marketplace.visualstudio.com/items?itemName=svelte.svelte-vscode).

## Need an official Svelte framework?

Check out [SvelteKit](https://github.com/sveltejs/kit#readme), which is also powered by Vite. Deploy anywhere with its serverless-first approach and adapt to various platforms, with out of the box support for TypeScript, SCSS, and Less, and easily-added support for mdsvex, GraphQL, PostCSS, Tailwind CSS, and more.

## Technical considerations

**Why use this over SvelteKit?**

- It brings its own routing solution which might not be preferable for some users.
- It is first and foremost a framework that just happens to use Vite under the hood, not a Vite app.

This template contains as little as possible to get started with Vite + Svelte, while taking into account the developer experience with regards to HMR and intellisense. It demonstrates capabilities on par with the other `create-vite` templates and is a good starting point for beginners dipping their toes into a Vite + Svelte project.

Should you later need the extended capabilities and extensibility provided by SvelteKit, the template has been structured similarly to SvelteKit so that it is easy to migrate.

**Why include `.vscode/extensions.json`?**

Other templates indirectly recommend extensions via the README, but this file allows VS Code to prompt the user to install the recommended extension upon opening the project.

**Why enable `checkJs` in the JS template?**

It is likely that most cases of changing variable types in runtime are likely to be accidental, rather than deliberate. This provides advanced typechecking out of the box. Should you like to take advantage of the dynamically-typed nature of JavaScript, it is trivial to change the configuration.

**Why is HMR not preserving my local component state?**

HMR state preservation comes with a number of gotchas! It has been disabled by default in both `svelte-hmr` and `@sveltejs/vite-plugin-svelte` due to its often surprising behavior. You can read the details [here](https://github.com/sveltejs/svelte-hmr/tree/master/packages/svelte-hmr#preservation-of-local-state).

If you have state that's important to retain within a component, consider creating an external store which would not be replaced by HMR.

```js
// store.js
// An extremely simple external store
import { writable } from 'svelte/store'
export default writable(0)
```

## Gestalt UI freeze troubleshooting

If tabs/buttons stop responding or the UI appears frozen, collect a crash report and the repro checklist.

### Enable sourcemaps for debug builds

- Dev mode: set `GESTALT_DEV_MODE=true` (for `gestalt --dev` runs).
- Explicit override: set `GESTALT_FRONTEND_SOURCEMAP=true` for non-dev builds.

Sourcemaps make stack traces readable but may expose source paths and content, so keep them off for production unless debugging.

### Crash reports

The frontend logs UI crashes to `POST /api/otel/logs` via `logUI`. Each crash record includes:

- crash id + session id
- active tab id + active view
- error message + stack
- URL and last successful refresh timestamps

Use the crash overlay to copy the crash id, then capture the details from the Logs UI.

### What to collect

- Timestamp, browser/OS, and build mode (dev/prod)
- Active tab/view and last action
- Console error + stack trace
- Last failing network request (status + payload shape)
- Crash id and session id (from the overlay)

See `frontend/docs/ui-freeze-triage.md` for the full checklist and triage decision tree.

## Vite chunking notes

The frontend keeps `@xterm/*` in its own chunk to avoid bloating the entry bundle.
Chunking rules live in `frontend/vite.config.js` (`manualChunks` -> `vendor-xterm`).
Non-default tab views (Plan/Flow/Terminal) are lazy-loaded in `frontend/src/App.svelte`, so avoid adding eager terminal imports at the root.
