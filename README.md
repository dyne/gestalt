# Gestalt documentation

The Dyne-styled VitePress documentation hub for Gestalt Agents and Gestalt
Mobile. It includes the onboarding journey, operational guides, copied source
documentation, a one-line installer, and the `gestalt` manager CLI.

```sh
npm ci
npm test
npm run build
```

Use `BASE_PATH=/gestalt/ npm run build` for the intended subpath deployment.

Pushes to `main` deploy through `.github/workflows/deploy-pages.yml`. In the
GitHub repository settings, set **Pages → Build and deployment → Source** to
**GitHub Actions**. The workflow obtains the repository's actual Pages base
path from `actions/configure-pages`, runs the shell tests, builds VitePress, and
deploys the generated artifact.
