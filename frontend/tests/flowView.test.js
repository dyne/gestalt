import { render, cleanup, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const fetchFlowActivities = vi.hoisted(() => vi.fn())
const fetchFlowConfig = vi.hoisted(() => vi.fn())
const fetchFlowEventTypes = vi.hoisted(() => vi.fn())
const saveFlowConfig = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  fetchFlowActivities,
  fetchFlowConfig,
  fetchFlowEventTypes,
  saveFlowConfig,
}))

import FlowView from '../src/views/FlowView.svelte'

describe('FlowView', () => {
  afterEach(() => {
    cleanup()
    fetchFlowActivities.mockReset()
    fetchFlowConfig.mockReset()
    fetchFlowEventTypes.mockReset()
    saveFlowConfig.mockReset()
  })

  it('filters triggers and updates the selected details', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({ eventTypes: ['workflow_paused', 'file_changed'] })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'workflow-paused',
            label: 'Workflow paused',
            event_type: 'workflow_paused',
            where: { session_id: 't1', agent_name: 'Codex' },
          },
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file_changed',
            where: { path: 'README.md' },
          },
        ],
        bindings_by_trigger_id: {},
      },
      temporalStatus: { enabled: true },
    })

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

  it('creates a trigger and saves it', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({
      eventTypes: ['workflow_paused', 'workflow_completed'],
    })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'workflow-paused',
            label: 'Workflow paused',
            event_type: 'workflow_paused',
            where: { session_id: 't1' },
          },
        ],
        bindings_by_trigger_id: {},
      },
      temporalStatus: { enabled: true },
    })
    saveFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'workflow-paused',
            label: 'Workflow paused',
            event_type: 'workflow_paused',
            where: { session_id: 't1' },
          },
          {
            id: 'new-trigger',
            label: 'New trigger',
            event_type: 'workflow_completed',
            where: { session_id: 't9' },
          },
        ],
        bindings_by_trigger_id: {},
      },
      temporalStatus: { enabled: true },
    })

    const { getByRole, getByLabelText, findAllByText } = render(FlowView)

    await findAllByText('Workflow paused')

    await fireEvent.click(getByRole('button', { name: 'Add trigger' }))

    await fireEvent.input(getByLabelText('Label'), { target: { value: 'New trigger' } })
    await fireEvent.change(getByLabelText('Event type'), { target: { value: 'workflow_completed' } })
    await fireEvent.input(getByLabelText('Where (one per line)'), { target: { value: 'session_id=t9' } })

    await fireEvent.click(getByRole('button', { name: 'Save trigger' }))

    expect((await findAllByText('New trigger')).length).toBeGreaterThan(0)
    expect(getByRole('heading', { level: 2, name: 'New trigger' })).toBeTruthy()

    await fireEvent.click(getByRole('button', { name: 'Save changes' }))

    expect(saveFlowConfig).toHaveBeenCalledTimes(1)
    expect(saveFlowConfig.mock.calls[0][0].triggers.some((trigger) => trigger.label === 'New trigger')).toBe(
      true,
    )
  })
})
