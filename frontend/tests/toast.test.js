import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'

import Toast from '../src/components/Toast.svelte'

describe('Toast', () => {
  beforeEach(() => {
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
  })

  afterEach(() => {
    cleanup()
  })

  it('dismisses on click', async () => {
    const onDismiss = vi.fn()
    const notification = {
      id: 'toast-1',
      level: 'warning',
      message: 'Heads up',
      autoClose: false,
      duration: 0,
    }

    const { getByText } = render(Toast, {
      props: { notification, onDismiss },
    })

    await fireEvent.click(getByText('Heads up'))
    expect(onDismiss).toHaveBeenCalledWith('toast-1')
  })

  it('dismisses on Enter', async () => {
    const onDismiss = vi.fn()
    const notification = {
      id: 'toast-2',
      level: 'info',
      message: 'Saved',
      autoClose: false,
      duration: 0,
    }

    const { container } = render(Toast, {
      props: { notification, onDismiss },
    })

    const toast = container.querySelector('.toast')
    await fireEvent.keyDown(toast, { key: 'Enter' })
    expect(onDismiss).toHaveBeenCalledWith('toast-2')
  })
})
