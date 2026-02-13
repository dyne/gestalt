import { describe, it, expect } from 'vitest'
import { buildTabs, resolveActiveView } from '../src/lib/tabRouting.js'

describe('tab routing', () => {
  it('includes the Agents home tab', () => {
    const tabs = buildTabs([])
    expect(tabs.find((tab) => tab.id === 'agents')?.label).toBe('Agents')
  })

  it('excludes external cli sessions from terminal tabs', () => {
    const tabs = buildTabs([
      { id: 'ext', title: 'External', interface: 'cli', runner: 'external' },
      { id: 'mcp', title: 'MCP', interface: 'mcp', runner: 'external' },
      { id: 'srv', title: 'Server', interface: 'cli', runner: 'server' },
    ])
    const ids = tabs.map((tab) => tab.id)
    expect(ids).not.toContain('ext')
    expect(ids).toContain('mcp')
    expect(ids).toContain('srv')
  })

  it('treats the Agents tab as a home view', () => {
    expect(resolveActiveView('agents')).toBe('agents')
  })
})
