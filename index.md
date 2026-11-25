---
layout: home

hero:
  name: "Codex</br>On The Road"
  text: "CLI & mobile"
  tagline: "Stop assembling the pieces and start perceiving the whole."
  image:
    src: https://dyne.org/images/logos/gestalt_logo.svg
    alt: Hands-on, Dyne.org style
  actions:
    - theme: brand
      text: Start in 5 minutes
      link: /start/
    - theme: alt
      text: Understand the system
      link: /agents/

features:
  - title: "Plan & execute"
    details: "A supervised Org-mode workflow with one active writer, explicit evidence gates, and reusable development skills."
  - title: "Save tokens"
    details: "Routes large command, log, file, and browser output through an indexed context: findings stay useful and compact."
  - title: "Work on the road"
    details: "A mobile-first web relay for durable Codex sessions, workspace navigation, plans, approvals, and Git operations."
  - title: "Resume in terminal"
    details: "Unified command-line tooling with session persistence, shell integration, and flexible interactions."
---
<HomeScreenshotCarousel />

<InstallCommand />

## Start your first session

From any directory, run `gestalt version` to check the installation. Run
`gestalt cli` to start a terminal session, or `gestalt mobile` to start the
web app and open the URL it prints. [Your first session](./start/first-session.md)

### Optional tools

On Linux or macOS, install Serena for code navigation or Playwright for browser automation.
Run the command for the tool you want.

```sh
gestalt serena install
gestalt playwright install
```

::: details Install inside an npm project
From the directory containing your `package.json`, install Gestalt, approve its
install script, and set up Agents.

```sh
npm i @jaromil/gestalt
npm install-scripts approve @jaromil/gestalt
npx gestalt agents install
```

Use `npx gestalt cli` to start a session in this project. To add an optional
tool on Linux or macOS, run either command below.

```sh
npx gestalt serena install
npx gestalt playwright install
```
:::
