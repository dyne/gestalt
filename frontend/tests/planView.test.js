import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import PlanView from '../src/views/PlanView.svelte'

describe('PlanView', () => {
  afterEach(() => {
    cleanup()
    apiFetch.mockReset()
  })

  it('renders plan content from the API', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({ content: '* TODO Hello' }),
    })

    const { findByText } = render(PlanView)

    expect(await findByText('Hello')).toBeTruthy()
    expect(apiFetch).toHaveBeenCalledWith('/api/plan')
  })
})
