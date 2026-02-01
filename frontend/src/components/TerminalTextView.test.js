import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

import TerminalTextView from './TerminalTextView.svelte'

const buildSegments = (text) => [{ kind: 'output', text }]

describe('TerminalTextView', () => {
  afterEach(() => {
    cleanup()
  })

  it('notifies when scrolled away from the bottom', async () => {
    const onAtBottomChange = vi.fn()
    const { container } = render(TerminalTextView, {
      props: {
        segments: buildSegments('one\ntwo\nthree'),
        onAtBottomChange,
      },
    })

    const body = container.querySelector('.terminal-text__body')
    expect(body).toBeTruthy()

    Object.defineProperty(body, 'scrollHeight', {
      configurable: true,
      value: 200,
    })
    Object.defineProperty(body, 'clientHeight', {
      configurable: true,
      value: 100,
    })

    body.scrollTop = 0
    await fireEvent.scroll(body)

    expect(onAtBottomChange).toHaveBeenCalledWith(false)
  })
})
