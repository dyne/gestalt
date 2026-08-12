import { defineConfig } from 'vitepress'

const base = process.env.BASE_PATH ?? '/'

export default defineConfig({
  title: 'Gestalt',
  titleTemplate: ':title · Gestalt',
  description: 'Documentation for Gestalt Agents and Gestalt Mobile by Dyne.org',
  lang: 'en-US',
  base,
  appearance: true,
  cleanUrls: true,
  // VitePress checks page routes, not extensionless/public download assets.
  ignoreDeadLinks: ['/install.sh', '/gestalt'],
  // Avoid invoking Git in source archives and restricted local previews. The
  // deployment checkout has history available and publishes real timestamps.
  lastUpdated: process.env.GITHUB_ACTIONS === 'true',
  head: [
    ['link', { rel: 'icon', href: `${base}favicon.png` }],
    ['meta', { name: 'theme-color', content: '#cb743b' }],
    ['meta', { name: 'color-scheme', content: 'light dark' }]
  ],
  markdown: {
    lineNumbers: true,
    toc: { level: [2, 3] }
  },
  themeConfig: {
    siteTitle: 'Gestalt',
    logo: { src: '/dyne-mark.svg', alt: 'Gestalt by Dyne.org' },
    search: {
      provider: 'local',
      options: {
        miniSearch: { searchOptions: { fuzzy: 0.2, prefix: true } }
      }
    },
    nav: [
      { text: 'Get started', link: '/start/' },
      { text: 'Agents', link: '/agents/' },
      { text: 'Mobile', link: '/mobile/' },
      { text: 'CLI', link: '/cli/' },
      { text: 'Reference', link: '/reference/' }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/dyne/gestalt-agents', ariaLabel: 'Gestalt Agents on GitHub' },
      { icon: 'github', link: 'https://github.com/dyne/gestalt-mobile', ariaLabel: 'Gestalt Mobile on GitHub' }
    ],
    sidebar: [
      {
        text: 'Welcome',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Choose your journey', link: '/start/' },
          { text: 'Install Gestalt', link: '/start/install' },
          { text: 'Your first session', link: '/start/first-session' }
        ]
      },
      {
        text: 'Gestalt CLI',
        items: [
          { text: 'Command reference', link: '/cli/' },
          { text: 'Configuration', link: '/cli/configuration' }
        ]
      },
      {
        text: 'Gestalt Agents',
        items: [
          { text: 'Introduction', link: '/agents/' },
          { text: 'Supervised workflow', link: '/agents/workflow' },
          { text: 'Skills', link: '/agents/skills' },
          { text: 'Context mode', link: '/agents/context-mode' }
        ]
      },
      {
        text: 'Gestalt Mobile',
        items: [
          { text: 'Introduction', link: '/mobile/' },
          { text: 'Sessions and Git', link: '/mobile/using' },
          { text: 'Network deployment', link: '/mobile/deployment' },
          { text: 'State and recovery', link: '/mobile/state-and-recovery' }
        ]
      },
      {
        text: 'Help and reference',
        items: [
          { text: 'Troubleshooting', link: '/troubleshooting' },
          { text: 'Source documentation', link: '/reference/' },
          { text: 'Contributing', link: '/contributing' }
        ]
      }
    ],
    outline: { level: [2, 3], label: 'On this page' },
    docFooter: { prev: 'Previous', next: 'Next' },
    lastUpdated: { text: 'Page updated' },
    externalLinkIcon: true,
    darkModeSwitchLabel: 'Appearance',
    sidebarMenuLabel: 'Documentation menu',
    returnToTopLabel: 'Return to top'
  }
})
