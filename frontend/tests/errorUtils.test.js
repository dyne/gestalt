import { describe, expect, it } from 'vitest'

import { getErrorMessage } from '../src/lib/errorUtils.js'

describe('errorUtils', () => {
  it('maps mcp bootstrap failures to actionable text', () => {
    const message = getErrorMessage({
      message: 'failed to start MCP session runtime',
      data: { code: 'mcp_bootstrap_failed' },
    })
    expect(message).toContain('MCP session failed to start')
  })
})
