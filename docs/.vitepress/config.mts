import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Gestalt',
  description: 'Multi-session dashboard for local coding agents and workflows.',
  lastUpdated: true,
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Getting Started', link: '/getting-started/quick-setup' },
      { text: 'Reference', link: '/reference/cli' },
      { text: 'Architecture', link: '/architecture-review' },
      { text: 'GitHub', link: 'https://github.com/dyne/gestalt' }
    ],
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Quick Setup', link: '/getting-started/quick-setup' },
          { text: 'Running On A Project', link: '/getting-started/running-on-a-project' }
        ]
      },
      {
        text: 'Guides',
        items: [
          { text: 'Build, Dev, And Testing', link: '/guides/build-dev-testing' },
          { text: 'Shutdown Behavior', link: '/shutdown' }
        ]
      },
      {
        text: 'Configuration',
        items: [
          { text: 'Agent Configuration', link: '/agent-configuration' },
          { text: 'Prompts, Skills, And Auth', link: '/configuration/prompts-skills-auth' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'CLI', link: '/reference/cli' },
          { text: 'HTTP API', link: '/reference/http-api' },
          { text: 'Events', link: '/notify-events' },
          { text: 'License', link: '/reference/license' }
        ]
      },
      {
        text: 'Architecture',
        items: [
          { text: 'Architecture Review', link: '/architecture-review' },
          { text: 'Event Bus', link: '/event-bus-architecture' },
          { text: 'Frontend Data Flow', link: '/frontend-data-flow' },
          { text: 'Go Dependencies', link: '/go-dependencies' },
          { text: 'OpenTelemetry', link: '/observability-otel-architecture' }
        ]
      }
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/dyne/gestalt' }],
    editLink: {
      pattern: 'https://github.com/dyne/gestalt/edit/main/docs/:path',
      text: 'Edit this page on GitHub'
    }
  }
})
