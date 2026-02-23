import { render, cleanup } from '@testing-library/svelte'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'

let Terminal

describe('Terminal', () => {
  afterEach(() => {
    cleanup()
  })

  beforeAll(async () => {
    if (typeof HTMLCanvasElement !== 'undefined') {
      HTMLCanvasElement.prototype.getContext = () => ({})
    }
    const module = await import('./Terminal.svelte')
    Terminal = module.default
  })

  it('does not render Plan button', async () => {
    const rendered = render(Terminal, {
      props: {
        sessionId: 'Coder 1',
      },
    })

    const button = rendered.queryByRole('button', { name: 'Plan' })
    expect(button).toBeNull()
  })
})
