import { describe, it, expect } from 'vitest'
import { parseOrg } from '../src/lib/orgParser.js'

describe('orgParser', () => {
  it('parses headings, keywords, priorities, and bodies into a tree', () => {
    const text = `* TODO [#A] First
Line one
Line two
** DONE Child task
Child body
* WIP Second
`
    const tree = parseOrg(text)

    expect(tree).toHaveLength(2)
    expect(tree[0]).toMatchObject({
      level: 1,
      keyword: 'TODO',
      priority: 'A',
      text: 'First',
    })
    expect(tree[0].body).toBe('Line one\nLine two')
    expect(tree[0].children).toHaveLength(1)
    expect(tree[0].children[0]).toMatchObject({
      level: 2,
      keyword: 'DONE',
      priority: '',
      text: 'Child task',
    })
    expect(tree[0].children[0].body).toBe('Child body')
    expect(tree[1]).toMatchObject({
      level: 1,
      keyword: 'WIP',
      priority: '',
      text: 'Second',
    })
  })

  it('handles headings without keywords or missing parents', () => {
    const text = `Intro text
** [#B] Orphan
Details
* Root`
    const tree = parseOrg(text)

    expect(tree).toHaveLength(2)
    expect(tree[0]).toMatchObject({
      level: 2,
      keyword: '',
      priority: 'B',
      text: 'Orphan',
    })
    expect(tree[0].body).toBe('Details')
    expect(tree[1]).toMatchObject({
      level: 1,
      keyword: '',
      priority: '',
      text: 'Root',
    })
  })
})
