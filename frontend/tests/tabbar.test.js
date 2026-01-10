import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

import TabBar from '../src/components/TabBar.svelte'

describe('TabBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('selects tabs', async () => {
    const onSelect = vi.fn()

    const { getByText, queryByLabelText } = render(TabBar, {
      props: {
        tabs: [
          { id: 'home', label: 'Home', isHome: true },
          { id: 't1', label: 'Shell', isHome: false },
        ],
        activeId: 'home',
        onSelect,
      },
    })

    await fireEvent.click(getByText('Shell'))
    expect(onSelect).toHaveBeenCalledWith('t1')
    expect(queryByLabelText('Close Shell')).toBeNull()
  })
})
