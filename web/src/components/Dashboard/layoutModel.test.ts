import { describe, expect, it } from 'vitest'
import { appendWidget, fromOrder, moveWidget, normalize, removeWidget, setWidth, widthOf, type Section } from './layoutModel'

const half = new Set(['a', 'b', 'c', 'd'])
const isHalf = (id: string) => half.has(id)
// Strips ids so layouts compare by shape.
const shape = (sections: Section[]) => sections.map((s) => s.columns)

const sec = (id: string, ...columns: string[][]): Section => ({ id, columns })

describe('fromOrder', () => {
  it('pairs consecutive half widgets and gives the rest their own row', () => {
    expect(shape(fromOrder(['full1', 'a', 'b', 'full2', 'c'], isHalf, []))).toEqual([
      [['full1']],
      [['a'], ['b']],
      [['full2']],
      [['c']],
    ])
  })

  it('skips hidden widgets before pairing', () => {
    expect(shape(fromOrder(['a', 'b', 'c'], isHalf, ['b']))).toEqual([[['a'], ['c']]])
  })

  it('does not pair halves separated by a full-width widget', () => {
    expect(shape(fromOrder(['a', 'full1', 'b'], isHalf, []))).toEqual([[['a']], [['full1']], [['b']]])
  })
})

describe('normalize', () => {
  it('drops unknown, duplicate and hidden ids, and empty rows', () => {
    const out = normalize(
      [sec('1', ['a', 'ghost'], ['a']), sec('2', ['b']), sec('3', [])],
      ['a', 'b', 'c'],
      ['b'],
    )
    // a once; b hidden; ghost unknown; empty row gone; c appended.
    expect(shape(out)).toEqual([[['a'], []], [['c']]])
  })

  it('appends widgets the layout has never seen, in known order', () => {
    expect(shape(normalize([sec('1', ['a'])], ['a', 'b', 'c'], []))).toEqual([[['a']], [['b']], [['c']]])
  })

  it('collapses a malformed row with more than two columns to its first', () => {
    const odd = [{ id: '1', columns: [['a'], ['b'], ['c']] }]
    expect(shape(normalize(odd, ['a', 'b', 'c'], []))).toEqual([[['a']], [['b']], [['c']]])
  })
})

describe('moveWidget', () => {
  const base = () => [sec('r1', ['a'], ['b']), sec('r2', ['c'])]

  it('stacks a widget under another in the same column', () => {
    const out = moveWidget(base(), 'c', { kind: 'column', sectionId: 'r1', column: 0, beforeId: null })
    expect(shape(out)).toEqual([[['a', 'c'], ['b']]]) // r2 emptied and removed
  })

  it('inserts before a given widget', () => {
    const out = moveWidget(base(), 'c', { kind: 'column', sectionId: 'r1', column: 1, beforeId: 'b' })
    expect(shape(out)).toEqual([[['a'], ['c', 'b']]])
  })

  it('reorders within a column', () => {
    const start = [sec('r1', ['a', 'b', 'c'])]
    const out = moveWidget(start, 'c', { kind: 'column', sectionId: 'r1', column: 0, beforeId: 'a' })
    expect(shape(out)).toEqual([[['c', 'a', 'b']]])
  })

  it('is a no-op when dropped on itself', () => {
    const start = base()
    expect(moveWidget(start, 'a', { kind: 'column', sectionId: 'r1', column: 0, beforeId: 'a' })).toBe(start)
  })

  it('moves the only widget of a row into its own empty column without losing it', () => {
    const start = [sec('r1', ['a'], [])]
    const out = moveWidget(start, 'a', { kind: 'column', sectionId: 'r1', column: 1, beforeId: null })
    expect(shape(out)).toEqual([[[], ['a']]])
  })

  it('creates a new full-width row at a gap, and removes the row it left', () => {
    const out = moveWidget(base(), 'b', { kind: 'gap', beforeSectionId: 'r2' })
    expect(shape(out)).toEqual([[['a'], []], [['b']], [['c']]])
  })

  it('creates a row at the end', () => {
    const out = moveWidget(base(), 'a', { kind: 'gap', beforeSectionId: null })
    expect(shape(out)).toEqual([[[], ['b']], [['c']], [['a']]])
  })

  it('leaves the layout alone when the target row no longer exists', () => {
    const start = base()
    expect(moveWidget(start, 'a', { kind: 'column', sectionId: 'gone', column: 0, beforeId: null })).toBe(start)
  })

  it('does not mutate its input', () => {
    const start = base()
    const snapshot = JSON.stringify(start)
    moveWidget(start, 'c', { kind: 'column', sectionId: 'r1', column: 0, beforeId: null })
    expect(JSON.stringify(start)).toBe(snapshot)
  })
})

describe('removeWidget / appendWidget', () => {
  it('removes a widget and the row it empties', () => {
    expect(shape(removeWidget([sec('r1', ['a'], ['b']), sec('r2', ['c'])], 'c'))).toEqual([[['a'], ['b']]])
  })

  it('appends a widget as a new full-width row', () => {
    expect(shape(appendWidget([sec('r1', ['a'])], 'b'))).toEqual([[['a']], [['b']]])
  })
})

describe('widthOf', () => {
  it('is half in a two-column row and full in a one-column row', () => {
    const layout = [sec('r1', ['a'], ['b']), sec('r2', ['c'])]
    expect(widthOf(layout, 'a')).toBe('half')
    expect(widthOf(layout, 'b')).toBe('half')
    expect(widthOf(layout, 'c')).toBe('full')
    expect(widthOf(layout, 'ghost')).toBeNull()
  })
})

describe('setWidth', () => {
  it('makes a half card full: it gets its own row right after its old one', () => {
    const out = setWidth([sec('r1', ['a'], ['b']), sec('r2', ['c'])], 'a', 'full')
    expect(shape(out)).toEqual([[[], ['b']], [['a']], [['c']]])
  })

  it('removes the old row when making its only card full leaves it empty', () => {
    const out = setWidth([sec('r1', ['a'], [])], 'a', 'full')
    expect(shape(out)).toEqual([[['a']]])
  })

  it('is a no-op when the card already has that width, or does not exist', () => {
    const layout = [sec('r1', ['a'], ['b']), sec('r2', ['c'])]
    expect(setWidth(layout, 'a', 'half')).toBe(layout)
    expect(setWidth(layout, 'c', 'full')).toBe(layout)
    expect(setWidth(layout, 'ghost', 'half')).toBe(layout)
  })

  it('makes a full card half by joining the shorter column of the row below', () => {
    const layout = [sec('r1', ['c']), sec('r2', ['a', 'x'], ['b'])]
    expect(shape(setWidth(layout, 'c', 'half'))).toEqual([[['a', 'x'], ['c', 'b']]])
  })

  it('joins the row above when there is none below', () => {
    const layout = [sec('r1', ['a'], ['b', 'y']), sec('r2', ['c'])]
    expect(shape(setWidth(layout, 'c', 'half'))).toEqual([[['a', 'c'], ['b', 'y']]])
  })

  it('prefers the row below when both neighbours are two-column', () => {
    const layout = [sec('r1', ['a'], ['b']), sec('r2', ['c']), sec('r3', ['d'], ['e'])]
    expect(shape(setWidth(layout, 'c', 'half'))).toEqual([[['a'], ['b']], [['c', 'd'], ['e']]])
  })

  it('fills a neighbouring empty slot first, so Full then Half restores the pairing', () => {
    const start = [sec('r1', ['a'], ['b']), sec('r2', ['c'], ['d'])]
    const full = setWidth(start, 'a', 'full') // a leaves r1, which now has an empty slot
    expect(shape(full)).toEqual([[[], ['b']], [['a']], [['c'], ['d']]])
    expect(shape(setWidth(full, 'a', 'half'))).toEqual([[['a'], ['b']], [['c'], ['d']]])
  })

  it('prefers an empty slot above over a fuller row below', () => {
    const layout = [sec('r1', [], ['b']), sec('r2', ['a']), sec('r3', ['c'], ['d'])]
    expect(shape(setWidth(layout, 'a', 'half'))).toEqual([[['a'], ['b']], [['c'], ['d']]])
  })

  it('becomes a half-width card awaiting a partner when no neighbour is two-column', () => {
    const layout = [sec('r1', ['a']), sec('r2', ['c']), sec('r3', ['d'])]
    expect(shape(setWidth(layout, 'c', 'half'))).toEqual([[['a']], [['c'], []], [['d']]])
  })

  it('lifts a card out of a stack of full-width cards', () => {
    const layout = [sec('r1', ['a', 'c']), sec('r2', ['d'], ['e'])]
    expect(shape(setWidth(layout, 'c', 'half'))).toEqual([[['a']], [['c', 'd'], ['e']]])
  })

  it('does not mutate its input', () => {
    const layout = [sec('r1', ['c']), sec('r2', ['a'], ['b'])]
    const snapshot = JSON.stringify(layout)
    setWidth(layout, 'c', 'half')
    expect(JSON.stringify(layout)).toBe(snapshot)
  })
})
