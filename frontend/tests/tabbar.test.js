import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

import TabBar from '../src/components/TabBar.svelte'

describe('TabBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('selects and closes tabs', async () => {
    const onSelect = vi.fn()
    const onClose = vi.fn()

    const { getByText, getByLabelText } = render(TabBar, {
      props: {
        tabs: [
          { id: 'home', label: 'Home', isHome: true },
          { id: 't1', label: 'Shell', isHome: false },
        ],
        activeId: 'home',
        onSelect,
        onClose,
      },
    })

    await fireEvent.click(getByText('Shell'))
    expect(onSelect).toHaveBeenCalledWith('t1')

    await fireEvent.click(getByLabelText('Close Shell'))
    expect(onClose).toHaveBeenCalledWith('t1')
  })
})
