import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { tick } from 'svelte'

import OrgViewer from '../src/components/OrgViewer.svelte'

describe('OrgViewer', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('collapses and expands nodes', async () => {
    const text = `* TODO [#A] Root
Body line
** DONE Child
Child body`
    const { getByText, queryByText } = render(OrgViewer, { props: { orgText: text } })

    expect(getByText('Root')).toBeTruthy()
    expect(getByText('Child')).toBeTruthy()

    await fireEvent.click(getByText('Collapse all'))
    expect(queryByText('Child')).toBeNull()
    expect(queryByText('Body line')).toBeNull()

    await fireEvent.click(getByText('Expand all'))
    expect(getByText('Child')).toBeTruthy()
  })

  it('expands matching nodes when searching', async () => {
    vi.useFakeTimers()
    const text = `* TODO Root
** WIP Child
Child body`
    const { getByText, getByPlaceholderText, queryByText } = render(OrgViewer, {
      props: { orgText: text },
    })

    await fireEvent.click(getByText('Collapse all'))
    expect(queryByText('Child body')).toBeNull()

    await fireEvent.input(getByPlaceholderText('Find headings or body text'), {
      target: { value: 'child body' },
    })
    vi.advanceTimersByTime(300)
    await tick()

    expect(getByText('Child body')).toBeTruthy()
  })
})
