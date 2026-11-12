// Pure layout logic for the dashboard grid, kept free of React so it can be
// tested directly. The dashboard is a list of rows ("sections"); each row has
// one or two columns, and each column is a vertical stack of widgets. Stacking
// small cards in one column is how a user evens out a tall card beside it.

export interface Section {
  id: string
  /** One column = full width; two columns sit side by side. */
  columns: string[][]
}

export type DropTarget =
  /** Put the widget in a column, before `beforeId` (null = at the end). */
  | { kind: 'column'; sectionId: string; column: number; beforeId: string | null }
  /** Put the widget in a new full-width row before `beforeSectionId` (null = at the end). */
  | { kind: 'gap'; beforeSectionId: string | null }

let counter = 0
export function newSectionId(): string {
  counter += 1
  return `s${Date.now().toString(36)}${counter.toString(36)}${Math.random().toString(36).slice(2, 5)}`
}

function cloneSections(sections: Section[]): Section[] {
  return sections.map((s) => ({ id: s.id, columns: s.columns.map((c) => [...c]) }))
}

/** Drops rows with nothing in them. */
function pruneEmpty(sections: Section[]): Section[] {
  return sections.filter((s) => s.columns.some((c) => c.length > 0))
}

/**
 * Builds a layout from a flat ordered list, the way the old dashboard did:
 * consecutive half-width widgets pair up into a two-column row, everything
 * else gets a full-width row of its own.
 */
export function fromOrder(order: string[], isHalf: (id: string) => boolean, hidden: string[]): Section[] {
  const sections: Section[] = []
  let open: Section | null = null // a lone half widget still waiting for a partner
  for (const id of order) {
    if (hidden.includes(id)) continue
    if (isHalf(id)) {
      if (open) {
        open.columns = [open.columns[0], [id]]
        open = null
      } else {
        open = { id: newSectionId(), columns: [[id]] }
        sections.push(open)
      }
    } else {
      open = null
      sections.push({ id: newSectionId(), columns: [[id]] })
    }
  }
  return sections
}

/**
 * Reconciles a saved layout with the current widget list: drops unknown,
 * duplicate and hidden ids, removes empty rows, and appends any widgets the
 * layout doesn't mention yet (in `known` order) as full-width rows so a newly
 * added widget shows up.
 */
export function normalize(sections: Section[], known: string[], hidden: string[]): Section[] {
  const knownSet = new Set(known)
  const seen = new Set<string>()
  const cleaned = sections.map((s) => ({
    id: s.id,
    columns: (s.columns.length === 2 ? s.columns : [s.columns[0] ?? []]).map((col) =>
      col.filter((id) => {
        if (!knownSet.has(id) || hidden.includes(id) || seen.has(id)) return false
        seen.add(id)
        return true
      }),
    ),
  }))
  const result = pruneEmpty(cleaned)
  for (const id of known) {
    if (!seen.has(id) && !hidden.includes(id)) result.push({ id: newSectionId(), columns: [[id]] })
  }
  return result
}

/** Moves a widget to a new position. Sections emptied by the move disappear. */
export function moveWidget(sections: Section[], widgetId: string, target: DropTarget): Section[] {
  if (target.kind === 'column' && target.beforeId === widgetId) return sections // dropped on itself
  const next = cloneSections(sections)

  for (const s of next) {
    for (const col of s.columns) {
      const i = col.indexOf(widgetId)
      if (i !== -1) col.splice(i, 1)
    }
  }

  if (target.kind === 'column') {
    const section = next.find((s) => s.id === target.sectionId)
    const col = section?.columns[target.column]
    if (!col) return sections // target vanished; leave the layout alone
    const at = target.beforeId === null ? -1 : col.indexOf(target.beforeId)
    if (at === -1) col.push(widgetId)
    else col.splice(at, 0, widgetId)
  } else {
    const row: Section = { id: newSectionId(), columns: [[widgetId]] }
    const at = target.beforeSectionId === null ? -1 : next.findIndex((s) => s.id === target.beforeSectionId)
    if (at === -1) next.push(row)
    else next.splice(at, 0, row)
  }
  return pruneEmpty(next)
}

/** Takes a widget out of the layout (e.g. when it is hidden). */
export function removeWidget(sections: Section[], widgetId: string): Section[] {
  return pruneEmpty(
    cloneSections(sections).map((s) => ({
      ...s,
      columns: s.columns.map((c) => c.filter((id) => id !== widgetId)),
    })),
  )
}

/** Adds a widget as a new full-width row at the end. */
export function appendWidget(sections: Section[], widgetId: string): Section[] {
  return [...cloneSections(sections), { id: newSectionId(), columns: [[widgetId]] }]
}

export type Width = 'full' | 'half'

/** A card is half width when it sits in a two-column row, otherwise full width. */
export function widthOf(sections: Section[], widgetId: string): Width | null {
  const section = sections.find((s) => s.columns.some((c) => c.includes(widgetId)))
  if (!section) return null
  return section.columns.length === 2 ? 'half' : 'full'
}

/**
 * Switches one card between full and half width.
 *
 * Full: the card leaves its two-column row and gets a full-width row right after it.
 * Half: the card joins a neighbouring two-column row. One with an empty slot (a card
 * waiting for a partner) wins, so switching to full and back restores the pairing;
 * otherwise the row below, then the row above, going into the shorter column. With no
 * such neighbour it becomes a half-width card in a new row, waiting for a partner.
 */
export function setWidth(sections: Section[], widgetId: string, width: Width): Section[] {
  const current = widthOf(sections, widgetId)
  if (current === null || current === width) return sections

  const next = cloneSections(sections)
  const at = next.findIndex((s) => s.columns.some((c) => c.includes(widgetId)))
  const section = next[at]
  for (const col of section.columns) {
    const i = col.indexOf(widgetId)
    if (i !== -1) col.splice(i, 1)
  }

  if (width === 'full') {
    next.splice(at + 1, 0, { id: newSectionId(), columns: [[widgetId]] })
    return pruneEmpty(next)
  }

  const shorter = (s: Section) => (s.columns[1].length < s.columns[0].length ? s.columns[1] : s.columns[0])
  const emptySlot = (s: Section) => (s.columns[0].length === 0 ? s.columns[0] : s.columns[1].length === 0 ? s.columns[1] : null)
  const below = next[at + 1]?.columns.length === 2 ? next[at + 1] : undefined
  const above = at > 0 && next[at - 1].columns.length === 2 ? next[at - 1] : undefined

  // A neighbouring row with an empty slot is waiting for a partner, such as the
  // row this card just left, so filling it undoes a switch to full width.
  const waiting = [above, below].find((s) => s && emptySlot(s))
  const waitingSlot = waiting ? emptySlot(waiting) : null
  if (waitingSlot) {
    waitingSlot.push(widgetId)
  } else if (below) {
    shorter(below).unshift(widgetId)
  } else if (above) {
    shorter(above).push(widgetId)
  } else {
    next.splice(at + 1, 0, { id: newSectionId(), columns: [[widgetId], []] })
  }
  return pruneEmpty(next)
}
