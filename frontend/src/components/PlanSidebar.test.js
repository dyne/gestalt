import { render, cleanup, waitFor } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'

import PlanSidebar from './PlanSidebar.svelte'

vi.mock('../lib/apiClient.js', () => ({
  fetchPlansList: vi.fn(),
  fetchSessionProgress: vi.fn(),
}))

vi.mock('../lib/terminalEventStore.js', () => ({
  subscribe: vi.fn(() => () => {}),
}))

const { fetchPlansList, fetchSessionProgress } = await import('../lib/apiClient.js')

describe('PlanSidebar', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('folds done L1 and highlights current L2', async () => {
    fetchSessionProgress.mockResolvedValue({
      has_progress: true,
      plan_file: 'plan.org',
      l1: 'Active L1',
      l2: 'Active L2',
      updated_at: '2026-02-12T00:00:00Z',
    })

    fetchPlansList.mockResolvedValue({
      plans: [
        {
          filename: 'plan.org',
          title: 'Plan Title',
          headings: [
            {
              keyword: 'DONE',
              text: 'Done L1',
              children: [{ keyword: 'DONE', text: 'Done L2', children: [] }],
            },
            {
              keyword: 'WIP',
              text: 'Active L1',
              children: [
                { keyword: 'TODO', text: 'Active L2', children: [] },
                { keyword: 'DONE', text: 'Finished L2', children: [] },
              ],
            },
          ],
        },
      ],
    })

    const { getByText, queryByText } = render(PlanSidebar, {
      props: {
        sessionId: 'Coder 1',
        open: true,
      },
    })

    await waitFor(() => {
      expect(getByText('Active L1')).toBeTruthy()
    })

    expect(queryByText('Done L2')).toBeNull()

    expect(getByText('Active L2')).toBeTruthy()
    expect(getByText('Finished L2')).toBeTruthy()

    const currentL2 = getByText('Active L2').closest('li')
    expect(currentL2?.getAttribute('data-current')).toBe('true')
  })
})
