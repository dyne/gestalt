import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach } from 'vitest'
import EventActivityAssignerHarness from '../../tests/helpers/EventActivityAssignerHarness.svelte'

const trigger = {
  id: 'trigger-1',
  label: 'Trigger',
  event_type: 'workflow_paused',
  where: {},
}

const activityDefs = [
  {
    id: 'toast_notification',
    label: 'Toast notification',
    description: 'Show a toast',
    fields: [{ key: 'message_template', label: 'Message', type: 'string' }],
  },
]

const createDataTransfer = () => {
  const data = {}
  return {
    data,
    setData(type, value) {
      data[type] = value
    },
    getData(type) {
      return data[type] || ''
    },
    effectAllowed: '',
  }
}

describe('EventActivityAssigner', () => {
  afterEach(() => cleanup())

  it('renders an empty state without a trigger', () => {
    const { getByText } = render(EventActivityAssignerHarness, {
      props: { trigger: null, activityDefs, bindings: [] },
    })
    expect(getByText('Select a trigger to assign activities.')).toBeTruthy()
  })

  it('dispatches assign events', async () => {
    const { getByRole, getByTestId } = render(EventActivityAssignerHarness, {
      props: {
        trigger,
        activityDefs,
        bindings: [],
      },
    })

    await fireEvent.click(getByRole('button', { name: 'Add Toast notification' }))
    expect(getByTestId('last-event').textContent).toBe('assign_activity')
    expect(JSON.parse(getByTestId('last-detail').textContent)).toEqual({
      trigger_id: 'trigger-1',
      activity_id: 'toast_notification',
      via: 'button',
    })

  })

  it('dispatches unassign events', async () => {
    const { getByRole, getByTestId } = render(EventActivityAssignerHarness, {
      props: {
        trigger,
        activityDefs,
        bindings: [{ activity_id: 'toast_notification', config: {} }],
      },
    })

    await fireEvent.click(getByRole('button', { name: 'Remove Toast notification' }))
    expect(getByTestId('last-event').textContent).toBe('unassign_activity')
    expect(JSON.parse(getByTestId('last-detail').textContent)).toEqual({
      trigger_id: 'trigger-1',
      activity_id: 'toast_notification',
      via: 'button',
    })
  })

  it('dispatches config updates', async () => {
    const { getByRole, getByLabelText, getByTestId } = render(EventActivityAssignerHarness, {
      props: {
        trigger,
        activityDefs,
        bindings: [{ activity_id: 'toast_notification', config: { message_template: 'hi' } }],
      },
    })

    await fireEvent.click(getByRole('button', { name: 'Configure Toast notification' }))
    await fireEvent.input(getByLabelText('Message'), { target: { value: 'hello' } })
    await fireEvent.click(getByRole('button', { name: 'Save' }))

    expect(getByTestId('last-event').textContent).toBe('update_activity_config')
    expect(JSON.parse(getByTestId('last-detail').textContent)).toEqual({
      trigger_id: 'trigger-1',
      activity_id: 'toast_notification',
      config: { message_template: 'hello' },
    })
  })

  it('dispatches drag-and-drop assign events', async () => {
    const { container, getByTestId } = render(EventActivityAssignerHarness, {
      props: {
        trigger,
        activityDefs,
        bindings: [],
      },
    })

    const draggable = container.querySelector('[data-activity-id="toast_notification"][data-source="available"]')
    const dropzone = container.querySelector('[data-dropzone="assigned"]')
    const dataTransfer = createDataTransfer()

    await fireEvent.dragStart(draggable, { dataTransfer })
    await fireEvent.dragOver(dropzone, { dataTransfer })
    await fireEvent.drop(dropzone, { dataTransfer })

    expect(getByTestId('last-event').textContent).toBe('assign_activity')
    expect(JSON.parse(getByTestId('last-detail').textContent)).toEqual({
      trigger_id: 'trigger-1',
      activity_id: 'toast_notification',
      via: 'dnd',
    })
  })

  it('dispatches drag-and-drop unassign events', async () => {
    const { container, getByTestId } = render(EventActivityAssignerHarness, {
      props: {
        trigger,
        activityDefs,
        bindings: [{ activity_id: 'toast_notification', config: {} }],
      },
    })

    const draggable = container.querySelector('[data-activity-id="toast_notification"][data-source="assigned"]')
    const dropzone = container.querySelector('[data-dropzone="available"]')
    const dataTransfer = createDataTransfer()

    await fireEvent.dragStart(draggable, { dataTransfer })
    await fireEvent.dragOver(dropzone, { dataTransfer })
    await fireEvent.drop(dropzone, { dataTransfer })

    expect(getByTestId('last-event').textContent).toBe('unassign_activity')
    expect(JSON.parse(getByTestId('last-detail').textContent)).toEqual({
      trigger_id: 'trigger-1',
      activity_id: 'toast_notification',
      via: 'dnd',
    })
  })
})
