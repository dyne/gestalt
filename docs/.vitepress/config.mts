import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Gestalt',
  description: 'Multi-session dashboard for local coding agents and workflows.',
  base: "/docs/gestalt/",
  head: [
    ['link', { rel: 'icon', href: 'https://dyne.org/images/logos/gestalt_logo.svg' }]
  ],
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started/overview' },
      { text: 'Reference', link: '/reference/cli' },
      { text: 'Architecture', link: '/architecture/architecture-review' },
      { text: 'GitHub', link: 'https://github.com/dyne/gestalt' }
    ],
    footer: {
      copyright:
        'Brought to you by <a href="https://dyne.org">Dyne.org</a> - 100% Free and open source',
    },
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Overview', link: '/getting-started/overview' },
          { text: 'Quick Setup', link: '/getting-started/quick-setup' },
          { text: 'Running On A Project', link: '/getting-started/running-on-a-project' }
        ]
      },
      {
        text: 'Guides',
        items: [
          { text: 'Build, Dev, And Testing', link: '/guides/build-dev-testing' },
          { text: 'Shutdown Behavior', link: '/guides/shutdown' }
        ]
      },
      {
        text: 'Configuration',
        items: [
          { text: 'Agent Configuration', link: '/configuration/agent-configuration' },
          { text: 'Prompts, Skills, And Auth', link: '/configuration/prompts-skills-auth' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI', link: '/reference/cli' },
          { text: 'HTTP API', link: '/reference/http-api' },
          { text: 'Events', link: '/reference/notify-events' },
          { text: 'License', link: '/reference/license' }
        ]
      },
      {
        text: 'Architecture',
        items: [
          { text: 'Architecture Review', link: '/architecture/architecture-review' },
          { text: 'Event Bus', link: '/architecture/event-bus-architecture' },
          { text: 'Frontend Data Flow', link: '/architecture/frontend-data-flow' },
          { text: 'Go Dependencies', link: '/architecture/go-dependencies' },
          { text: 'OpenTelemetry', link: '/architecture/observability-otel-architecture' }
        ]
      }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/dyne/gestalt' },
      { icon: "maildotru", link: "mailto:info@dyne.org" },
      { icon: "linkedin", link: "https://www.linkedin.com/company/dyne-org" }
    ]
  }
});