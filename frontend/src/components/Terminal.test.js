import { render, fireEvent, cleanup } from '@testing-library/svelte'
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

  it('shows the Plan button for coder role and toggles open state', async () => {
    let open = false
    let rerender = async () => {}
    const onTogglePlan = async () => {
      open = !open
      await rerender({
        role: 'Coder',
        planSidebarOpen: open,
        onTogglePlan,
      })
    }

    const rendered = render(Terminal, {
      props: {
        role: 'Coder',
        planSidebarOpen: open,
        onTogglePlan,
      },
    })
    rerender = rendered.rerender

    const button = rendered.getByRole('button', { name: 'Plan' })
    expect(button).toBeTruthy()
    expect(button.getAttribute('aria-pressed')).toBe('false')

    await fireEvent.click(button)
    expect(button.getAttribute('aria-pressed')).toBe('true')
  })
})
