import { render, cleanup } from '@testing-library/svelte'
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
})
