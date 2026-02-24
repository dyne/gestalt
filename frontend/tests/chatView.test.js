import { render, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

vi.mock('../src/components/DirectorComposer.svelte', async () => {
  const module = await import('./helpers/DirectorComposerMock.svelte')
  return { default: module.default }
})

import ChatView from '../src/views/ChatView.svelte'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ChatView', () => {
  it('renders chat bubbles in order', async () => {
    const { findByText, container } = render(ChatView, {
      props: {
        messages: [
          { id: '1', role: 'user', text: 'hello' },
          { id: '2', role: 'assistant', text: 'world' },
        ],
      },
    })

    expect(await findByText('hello')).toBeTruthy()
    expect(await findByText('world')).toBeTruthy()
    expect(container.querySelector('.chat-view')?.className).toContain('home-surface--base')
  })

  it('forwards composer submit callback', async () => {
    const onDirectorSubmit = vi.fn()
    const { findByRole } = render(ChatView, {
      props: {
        messages: [],
        onDirectorSubmit,
      },
    })

    const submit = await findByRole('button', { name: 'Mock submit' })
    await fireEvent.click(submit)

    expect(onDirectorSubmit).toHaveBeenCalledWith({ text: 'mock text', source: 'text' })
  })

  it('shows empty and streaming states', async () => {
    const { findByText } = render(ChatView, {
      props: {
        messages: [],
        streaming: true,
      },
    })

    expect(await findByText('Start by sending a message to Director.')).toBeTruthy()
    expect(await findByText('Director is responding…')).toBeTruthy()
  })

  it('renders chat content on narrow viewports', async () => {
    Object.defineProperty(window, 'innerWidth', {
      value: 640,
      writable: true,
      configurable: true,
    })
    window.dispatchEvent(new Event('resize'))

    const { findByText } = render(ChatView, {
      props: {
        messages: [{ id: '1', role: 'assistant', text: 'mobile message' }],
      },
    })
    expect(await findByText('mobile message')).toBeTruthy()
  })
})
