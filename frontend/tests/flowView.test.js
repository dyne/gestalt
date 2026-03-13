import { render, cleanup, fireEvent, waitFor } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const fetchFlowActivities = vi.hoisted(() => vi.fn())
const fetchFlowConfig = vi.hoisted(() => vi.fn())
const fetchFlowEventTypes = vi.hoisted(() => vi.fn())
const fetchTerminals = vi.hoisted(() => vi.fn())
const saveFlowConfig = vi.hoisted(() => vi.fn())
const exportFlowConfig = vi.hoisted(() => vi.fn())
const importFlowConfig = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  fetchFlowActivities,
  fetchFlowConfig,
  fetchFlowEventTypes,
  fetchTerminals,
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
    fetchTerminals.mockReset()
    saveFlowConfig.mockReset()
    exportFlowConfig.mockReset()
    importFlowConfig.mockReset()
  })

  it('filters triggers and updates the selected details', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({ eventTypes: ['file-change', 'git-branch'] })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file-change',
            where: { path: 'README.md' },
          },
          {
            id: 'git-branch',
            label: 'Git branch changed',
            event_type: 'git-branch',
            where: { branch: 'main' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })
    fetchTerminals.mockResolvedValue([])

    const { getByLabelText, getByRole, queryByText, findAllByText, findByText, container } = render(FlowView)

    expect((await findAllByText('File changed')).length).toBeGreaterThan(0)
    expect(await findByText('Git branch changed')).toBeTruthy()

    const input = getByLabelText('Search / filters')
    await fireEvent.input(input, { target: { value: 'event_type:file-change' } })

    expect((await findAllByText('File changed')).length).toBeGreaterThan(0)
    expect(queryByText('Git branch changed')).toBeNull()

    await fireEvent.input(input, { target: { value: '' } })

    const branchTrigger = await findByText('Git branch changed')
    await fireEvent.click(branchTrigger)

    const heading = getByRole('heading', { level: 2, name: 'Git branch changed' })
    expect(heading).toBeTruthy()
    expect(container.querySelector('.flow-view')?.className).toContain('home-surface--base')
  })

  it('creates a trigger and saves it', async () => {
    fetchFlowActivities.mockResolvedValue([{ id: 'toast_notification', label: 'Toast', fields: [] }])
    fetchFlowEventTypes.mockResolvedValue({
      eventTypes: ['file-change', 'git-commit'],
    })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file-change',
            where: { path: 'README.md' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })
    fetchTerminals.mockResolvedValue([])
    saveFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [
          {
            id: 'file-changed',
            label: 'File changed',
            event_type: 'file-change',
            where: { path: 'README.md' },
          },
          {
            id: 'new-trigger',
            label: 'New trigger',
            event_type: 'git-commit',
            where: { commit_hash: 't9' },
          },
        ],
        bindings_by_trigger_id: {},
      },
    })

    const { getByRole, getByLabelText, findAllByText } = render(FlowView)

    await findAllByText('File changed')

    await fireEvent.click(getByRole('button', { name: 'Add trigger' }))

    await fireEvent.input(getByLabelText('Label'), { target: { value: 'New trigger' } })
    await fireEvent.change(getByLabelText('Event type'), { target: { value: 'git-commit' } })
    await fireEvent.input(getByLabelText('Where (one per line)'), { target: { value: 'commit_hash=t9' } })

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
      eventTypes: ['file-change'],
    })
    fetchFlowConfig.mockResolvedValue({
      config: {
        version: 1,
        triggers: [],
        bindings_by_trigger_id: {},
      },
    })
    fetchTerminals.mockResolvedValue([])
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
