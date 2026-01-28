import { render, cleanup, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, afterEach } from 'vitest'

import FlowView from '../src/views/FlowView.svelte'

describe('FlowView', () => {
  afterEach(() => cleanup())

  it('filters triggers and updates the selected details', async () => {
    const { getByLabelText, getByRole, queryByText, findAllByText, findByText } = render(FlowView)

    expect((await findAllByText('Workflow paused')).length).toBeGreaterThan(0)
    expect(await findByText('File changed')).toBeTruthy()

    const input = getByLabelText('Search / filters')
    await fireEvent.input(input, { target: { value: 'event_type:workflow_paused' } })

    expect((await findAllByText('Workflow paused')).length).toBeGreaterThan(0)
    expect(queryByText('File changed')).toBeNull()

    await fireEvent.input(input, { target: { value: '' } })

    const fileTrigger = await findByText('File changed')
    await fireEvent.click(fileTrigger)

    const heading = getByRole('heading', { level: 2, name: 'File changed' })
    expect(heading).toBeTruthy()
  })

  it('creates a trigger and selects it', async () => {
    const { getByRole, getByLabelText, findAllByText } = render(FlowView)

    await fireEvent.click(getByRole('button', { name: 'Add trigger' }))

    await fireEvent.input(getByLabelText('Label'), { target: { value: 'New trigger' } })
    await fireEvent.change(getByLabelText('Event type'), { target: { value: 'workflow_completed' } })
    await fireEvent.input(getByLabelText('Where (one per line)'), { target: { value: 'terminal_id=t9' } })

    await fireEvent.click(getByRole('button', { name: 'Save trigger' }))

    expect((await findAllByText('New trigger')).length).toBeGreaterThan(0)
    expect(getByRole('heading', { level: 2, name: 'New trigger' })).toBeTruthy()
  })
})
