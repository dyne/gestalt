import { describe, it, expect } from 'vitest'
import { buildTabs, resolveActiveView } from '../src/lib/tabRouting.js'

describe('tab routing', () => {
  it('includes the Agents home tab when enabled', () => {
    const tabs = buildTabs([], { showAgents: true })
    expect(tabs.find((tab) => tab.id === 'agents')?.label).toBe('Agents')
  })

  it('omits the Agents home tab by default', () => {
    const tabs = buildTabs([])
    expect(tabs.find((tab) => tab.id === 'agents')).toBeUndefined()
  })

  it('omits the Chat home tab by default', () => {
    const tabs = buildTabs([])
    expect(tabs.find((tab) => tab.id === 'chat')).toBeUndefined()
  })

  it('includes the Chat home tab when enabled', () => {
    const tabs = buildTabs([], { showChat: true })
    expect(tabs.find((tab) => tab.id === 'chat')?.label).toBe('Chat')
  })

  it('excludes cli sessions from terminal tabs', () => {
    const tabs = buildTabs([
      { id: 'ext', title: 'External', interface: 'cli', runner: 'external' },
      { id: 'legacy', title: 'Legacy', interface: 'legacy', runner: 'external' },
      { id: 'srv', title: 'Server', interface: 'cli', runner: 'server' },
    ])
    const ids = tabs.map((tab) => tab.id)
    expect(ids).not.toContain('ext')
    expect(ids).toContain('legacy')
    expect(ids).not.toContain('srv')
  })

  it('treats the Agents tab as a home view', () => {
    expect(resolveActiveView('agents')).toBe('agents')
  })

  it('treats the Chat tab as a home view', () => {
    expect(resolveActiveView('chat')).toBe('chat')
  })
})
