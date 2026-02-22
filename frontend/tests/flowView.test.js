import { render, cleanup, fireEvent, waitFor } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const fetchFlowActivities = vi.hoisted(() => vi.fn())
const fetchFlowConfig = vi.hoisted(() => vi.fn())
const fetchFlowEventTypes = vi.hoisted(() => vi.fn())
const saveFlowConfig = vi.hoisted(() => vi.fn())
const exportFlowConfig = vi.hoisted(() => vi.fn())
const importFlowConfig = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  fetchFlowActivities,
  fetchFlowConfig,
  fetchFlowEventTypes,
  saveFlowConfig,
  exportFlowConfig,
  importFlowConfig,
}))

import FlowView from '../src/views/FlowView.svelte'

describe('FlowView', () => {
  afterEach(() => {
    cleanup()
    fetchFlowActivities.mockReset()
    fetchFlowConfig.mockReset()
    fetchFlowEventTypes.mockReset()
    saveFlowConfig.mockReset()
    exportFlowConfig.mockReset()
    importFlowConfig.mockReset()
  })

  it('filters triggers and updates the selected details', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({ eventTypes: ['file_changed', 'git_branch_changed'] })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file_changed',
            where: { path: 'README.md' },
          },
          {
            id: 'git-branch',
            label: 'Git branch changed',
            event_type: 'git_branch_changed',
            where: { branch: 'main' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })

    const { getByLabelText, getByRole, queryByText, findAllByText, findByText } = render(FlowView)

    expect((await findAllByText('File changed')).length).toBeGreaterThan(0)
    expect(await findByText('Git branch changed')).toBeTruthy()

    const input = getByLabelText('Search / filters')
    await fireEvent.input(input, { target: { value: 'event_type:file_changed' } })

    expect((await findAllByText('File changed')).length).toBeGreaterThan(0)
    expect(queryByText('Git branch changed')).toBeNull()

    await fireEvent.input(input, { target: { value: '' } })

    const branchTrigger = await findByText('Git branch changed')
    await fireEvent.click(branchTrigger)

    const heading = getByRole('heading', { level: 2, name: 'Git branch changed' })
    expect(heading).toBeTruthy()
  })

  it('creates a trigger and saves it', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({
      eventTypes: ['file_changed', 'terminal_resized'],
    })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file_changed',
            where: { path: 'README.md' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })
    saveFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file_changed',
            where: { path: 'README.md' },
          },
          {
            id: 'new-trigger',
            label: 'New trigger',
            event_type: 'terminal_resized',
            where: { terminal_id: 't9' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })

    const { getByRole, getByLabelText, findAllByText } = render(FlowView)

    await findAllByText('File changed')

    await fireEvent.click(getByRole('button', { name: 'Add trigger' }))

    await fireEvent.input(getByLabelText('Label'), { target: { value: 'New trigger' } })
    await fireEvent.change(getByLabelText('Event type'), { target: { value: 'terminal_resized' } })
    await fireEvent.input(getByLabelText('Where (one per line)'), { target: { value: 'terminal_id=t9' } })

    await fireEvent.click(getByRole('button', { name: 'Save trigger' }))

    expect((await findAllByText('New trigger')).length).toBeGreaterThan(0)
    expect(getByRole('heading', { level: 2, name: 'New trigger' })).toBeTruthy()

    await fireEvent.click(getByRole('button', { name: 'Save changes' }))

    expect(saveFlowConfig).toHaveBeenCalledTimes(1)
    expect(saveFlowConfig.mock.calls[0][0].triggers.some((trigger) => trigger.label === 'New trigger')).toBe(
      true,
    )
  })

  it('imports yaml flow files as raw text', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({
      eventTypes: ['file_changed'],
    })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [],
        bindings_by_trigger_id: {},
      },
    })
    importFlowConfig.mockResolvedValue({
      config: { version: 1, triggers: [], bindings_by_trigger_id: {} },
    })

    const { container, findByText } = render(FlowView)
    await findByText('Flow')

    const input = container.querySelector('.flow-import-input')
    expect(input).toBeTruthy()
    expect(input.getAttribute('accept')).toBe('.yaml,.yml')

    const file = {
      name: 'flows.yaml',
      type: 'text/yaml',
      text: vi.fn().mockResolvedValue('version: 1\nflows: []\n'),
    }
    await fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => {
      expect(importFlowConfig).toHaveBeenCalledWith('version: 1\nflows: []\n')
    })
  })
})
