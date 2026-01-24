import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach } from 'vitest'

import ViewStateHarness from './helpers/ViewStateHarness.svelte'

describe('ViewState', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows loading when no content', () => {
    const { getByText } = render(ViewStateHarness, {
      props: {
        loading: true,
        hasContent: false,
        loadingLabel: 'Loading data...',
      },
    })

    expect(getByText('Loading data...')).toBeTruthy()
  })

  it('shows error when empty', () => {
    const { getByText } = render(ViewStateHarness, {
      props: {
        loading: false,
        error: 'Boom',
        hasContent: false,
      },
    })

    expect(getByText('Boom')).toBeTruthy()
  })

  it('renders content with inline error', () => {
    const { getByText } = render(ViewStateHarness, {
      props: {
        loading: false,
        error: 'Oops',
        hasContent: true,
        content: 'Content',
      },
    })

    expect(getByText('Oops')).toBeTruthy()
    expect(getByText('Content')).toBeTruthy()
  })

  it('renders content even when empty if showEmpty is false', () => {
    const { getByText } = render(ViewStateHarness, {
      props: {
        loading: false,
        error: '',
        hasContent: false,
        showEmpty: false,
        content: 'Fallback',
      },
    })

    expect(getByText('Fallback')).toBeTruthy()
  })
})
