import { render, cleanup, waitFor } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const subscribeEvents = vi.hoisted(() => vi.fn(() => () => {}))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/lib/eventStore.js', () => ({
  subscribe: subscribeEvents,
}))

import PlanView from '../src/views/PlanView.svelte'

describe('PlanView', () => {
  afterEach(() => {
    cleanup()
    apiFetch.mockReset()
    subscribeEvents.mockReset()
  })

  it('renders plans from the API', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        plans: [
          {
            filename: '2026-01-01-sample.org',
            title: 'Sample Plan',
            subtitle: 'Example',
            date: '2026-01-01',
            l1_count: 1,
            l2_count: 0,
            priority_a: 0,
            priority_b: 0,
            priority_c: 0,
            headings: [
              {
                level: 1,
                keyword: 'TODO',
                priority: 'A',
                text: 'First L1',
                body: '',
                children: [],
              },
            ],
          },
        ],
      }),
    })

    const { findByText } = render(PlanView)

    expect(await findByText('Sample Plan')).toBeTruthy()
    expect(apiFetch).toHaveBeenCalledWith('/api/plans')
  })

  it('refreshes plans with duplicate headings safely', async () => {
    let handler = null
    subscribeEvents.mockImplementation((type, callback) => {
      handler = callback
      return () => {}
    })

    apiFetch
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue({
          plans: [
            {
              filename: 'dup-plan.org',
              title: 'Duplicate Plan',
              headings: [
                {
                  level: 1,
                  keyword: 'TODO',
                  priority: 'A',
                  text: 'Repeat',
                  body: '',
                  children: [
                    {
                      level: 2,
                      keyword: 'TODO',
                      priority: 'B',
                      text: 'Repeat',
                      body: '',
                      children: [],
                    },
                  ],
                },
              ],
            },
          ],
        }),
      })
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue({
          plans: [
            {
              filename: 'dup-plan.org',
              title: 'Duplicate Plan',
              headings: [
                {
                  level: 1,
                  keyword: 'WIP',
                  priority: 'B',
                  text: 'Repeat',
                  body: '',
                  children: [],
                },
                {
                  level: 1,
                  keyword: 'TODO',
                  priority: 'A',
                  text: 'Repeat',
                  body: '',
                  children: [],
                },
              ],
            },
          ],
        }),
      })

    const { findByText } = render(PlanView)

    expect(await findByText('Duplicate Plan')).toBeTruthy()

    handler?.({ path: '/repo/.gestalt/plans/dup-plan.org' })

    await new Promise((resolve) => setTimeout(resolve, 300))

    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledTimes(2)
    })
  })
})
