import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach } from 'vitest'
import PlanCard from './PlanCard.svelte'

describe('PlanCard', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders summary metadata and stats', () => {
    const plan = {
      filename: '2026-01-01-plan.org',
      title: 'Plan Title',
      subtitle: 'Short subtitle',
      date: '2026-01-01',
      l1_count: 2,
      l2_count: 3,
      priority_a: 1,
      priority_b: 0,
      priority_c: 2,
      headings: [],
    }

    const { getByText, queryByText } = render(PlanCard, { props: { plan } })

    expect(getByText('Plan Title')).toBeTruthy()
    expect(getByText('Short subtitle')).toBeTruthy()
    expect(getByText('2026-01-01')).toBeTruthy()
    expect(getByText('L1')).toBeTruthy()
    expect(getByText('L2')).toBeTruthy()
    expect(getByText('[#A] 1')).toBeTruthy()
    expect(getByText('[#C] 2')).toBeTruthy()
    expect(queryByText('[#B]')).toBeNull()
  })

  it('renders nested L1 and L2 details', () => {
    const plan = {
      filename: '2026-01-01-plan.org',
      title: 'Plan Title',
      subtitle: '',
      date: '',
      l1_count: 1,
      l2_count: 1,
      priority_a: 0,
      priority_b: 1,
      priority_c: 0,
      headings: [
        {
          level: 1,
          keyword: 'WIP',
          priority: 'A',
          text: 'First L1',
          body: 'L1 body text',
          children: [
            {
              level: 2,
              keyword: 'TODO',
              priority: 'B',
              text: 'First L2',
              body: 'L2 body text',
              children: [],
            },
          ],
        },
      ],
    }

    const { getByText } = render(PlanCard, { props: { plan } })

    expect(getByText('First L1')).toBeTruthy()
    expect(getByText('L1 body text')).toBeTruthy()
    expect(getByText('First L2')).toBeTruthy()
    expect(getByText('L2 body text')).toBeTruthy()
  })

  it('handles duplicate headings without throwing', async () => {
    const plan = {
      filename: '2026-01-01-plan.org',
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
        {
          level: 1,
          keyword: 'TODO',
          priority: 'B',
          text: 'Repeat',
          body: '',
          children: [],
        },
      ],
    }

    const { rerender, getAllByText } = render(PlanCard, { props: { plan } })

    expect(getAllByText('Repeat').length).toBeGreaterThan(1)

    await rerender({
      plan: {
        ...plan,
        headings: [
          ...plan.headings,
          {
            level: 1,
            keyword: 'WIP',
            priority: 'C',
            text: 'Repeat',
            body: 'Extra heading',
            children: [],
          },
        ],
      },
    })

    expect(getAllByText('Repeat').length).toBeGreaterThan(1)
  })
})
