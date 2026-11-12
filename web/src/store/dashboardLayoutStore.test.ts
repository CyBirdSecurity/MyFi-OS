import { beforeEach, describe, expect, it, vi } from 'vitest'

const KEY = 'myfi-dashboard-layout'

// The store persists to window.localStorage, which doesn't exist under vitest's
// default node environment, so give it an in-memory one before importing.
function installStorage(initial?: Record<string, string>) {
  const data = new Map(Object.entries(initial ?? {}))
  const localStorage = {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
    removeItem: (k: string) => void data.delete(k),
  }
  vi.stubGlobal('localStorage', localStorage)
  vi.stubGlobal('window', { localStorage })
}

async function freshStore() {
  vi.resetModules()
  const mod = await import('./dashboardLayoutStore')
  return mod
}

const shape = (sections: { columns: string[][] }[]) => sections.map((s) => s.columns)

const DEFAULT_SHAPE = [
  [['summary']],
  [['netWorthChart']],
  [['spendingSummary'], ['goals']],
  [['allocation'], ['household', 'readiness']],
  [['recentTransactions'], ['debtPayoff', 'mortgagePayoff']],
  [['taxBreakdown']],
]

// Cards that start out of the dashboard, available under Add card.
const DEFAULT_HIDDEN = ['topMovers', 'cashFlow', 'portfolioCheckup']

const placedIds = (sections: { columns: string[][] }[]) => sections.flatMap((s) => s.columns.flat())

beforeEach(() => {
  installStorage()
})

describe('default dashboard layout', () => {
  it('is the arrangement new users start with, with the optional cards hidden', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect(shape(sections)).toEqual(DEFAULT_SHAPE)
    expect([...hidden].sort()).toEqual([...DEFAULT_HIDDEN].sort())
  })

  it('places every widget exactly once, as a card or in the hidden tray', async () => {
    const { useDashboardLayoutStore, DASHBOARD_WIDGETS } = await freshStore()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    const placed = [...placedIds(sections), ...hidden].sort()
    expect(placed).toEqual(DASHBOARD_WIDGETS.map((w) => w.id).sort())
  })

  it('gives every row a unique id', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    const ids = useDashboardLayoutStore.getState().sections.map((s) => s.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  // The load step runs even when nothing is saved; it must not wipe the
  // default hidden cards for someone who has never customized.
  it('survives loading for a user with nothing saved', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect(shape(sections)).toEqual(DEFAULT_SHAPE)
    expect([...hidden].sort()).toEqual([...DEFAULT_HIDDEN].sort())
  })

  it('is restored by Reset after the user rearranges and un-hides cards', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    const store = useDashboardLayoutStore.getState()
    store.showWidget('topMovers')
    store.showWidget('cashFlow')
    store.hideWidget('goals')
    expect(useDashboardLayoutStore.getState().hidden).toEqual(['portfolioCheckup', 'goals'])

    useDashboardLayoutStore.getState().reset()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect(shape(sections)).toEqual(DEFAULT_SHAPE)
    expect([...hidden].sort()).toEqual([...DEFAULT_HIDDEN].sort())
  })
})

describe('a saved layout', () => {
  it('wins over the default, including a card the user chose to show', async () => {
    installStorage({
      [KEY]: JSON.stringify({
        version: 2,
        state: {
          hidden: [],
          // Top Movers and Cash Flow are placed, so the user has them on.
          sections: [{ id: 's1', columns: [['summary', 'topMovers', 'cashFlow']] }],
        },
      }),
    })
    const { useDashboardLayoutStore, DASHBOARD_WIDGETS } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect(sections[0].columns).toEqual([['summary', 'topMovers', 'cashFlow']])
    // Portfolio Check-up isn't in the saved layout, so it stays opt-in.
    expect(hidden).toEqual(['portfolioCheckup'])
    // Every other card the saved layout didn't mention is appended rather than lost.
    expect([...placedIds(sections), ...hidden].sort()).toEqual(DASHBOARD_WIDGETS.map((w) => w.id).sort())
  })

  it('keeps cards added to the app later hidden, rather than adding them unasked', async () => {
    // A layout saved before Cash Flow and Portfolio Check-up existed.
    installStorage({
      [KEY]: JSON.stringify({
        version: 2,
        state: { hidden: ['topMovers'], sections: [{ id: 's1', columns: [['summary']] }] },
      }),
    })
    const { useDashboardLayoutStore } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect([...hidden].sort()).toEqual([...DEFAULT_HIDDEN].sort())
    expect(placedIds(sections)).not.toContain('cashFlow')
    expect(placedIds(sections)).not.toContain('portfolioCheckup')
  })

  it('does not hide an optional card the user already has on their dashboard', async () => {
    installStorage({
      [KEY]: JSON.stringify({
        version: 2,
        state: { hidden: [], sections: [{ id: 's1', columns: [['summary'], ['topMovers']] }] },
      }),
    })
    const { useDashboardLayoutStore } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    expect(useDashboardLayoutStore.getState().hidden).not.toContain('topMovers')
  })

  it('migrates a layout from the old ordered list, keeping order and hidden cards', async () => {
    installStorage({
      [KEY]: JSON.stringify({
        version: 0,
        state: { order: ['goals', 'topMovers', 'summary'], hidden: ['allocation'] },
      }),
    })
    const { useDashboardLayoutStore } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    const { sections, hidden } = useDashboardLayoutStore.getState()
    expect(hidden).toEqual(expect.arrayContaining(['allocation', 'cashFlow', 'portfolioCheckup']))
    // Top Movers was in the old list, so it stays on the dashboard.
    expect(hidden).not.toContain('topMovers')
    expect(placedIds(sections).slice(0, 3)).toEqual(['goals', 'topMovers', 'summary'])
    expect(placedIds(sections)).not.toContain('cashFlow')
  })
})

describe('card options', () => {
  it('start at each option\'s default', async () => {
    const { useDashboardLayoutStore, optionValue } = await freshStore()
    expect(optionValue(useDashboardLayoutStore.getState().options, 'netWorthChart', 'view')).toBe('simple')
  })

  it('can be switched and are saved with the layout', async () => {
    const { useDashboardLayoutStore, optionValue } = await freshStore()
    useDashboardLayoutStore.getState().setOption('netWorthChart', 'view', 'detailed')
    expect(optionValue(useDashboardLayoutStore.getState().options, 'netWorthChart', 'view')).toBe('detailed')

    // A fresh load of the saved state keeps the choice.
    const saved = JSON.parse(localStorage.getItem(KEY)!)
    expect(saved.state.options).toEqual({ 'netWorthChart.view': 'detailed' })
    const reloaded = await freshStore()
    await reloaded.useDashboardLayoutStore.persist.rehydrate()
    expect(optionValue(reloaded.useDashboardLayoutStore.getState().options, 'netWorthChart', 'view')).toBe('detailed')
  })

  it('ignore a value the card does not offer, and a card with no such option', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    useDashboardLayoutStore.getState().setOption('netWorthChart', 'view', 'fancy')
    useDashboardLayoutStore.getState().setOption('goals', 'view', 'detailed')
    useDashboardLayoutStore.getState().setOption('netWorthChart', 'nope', 'detailed')
    expect(useDashboardLayoutStore.getState().options).toEqual({})
  })

  it('drop stale saved values on load instead of trusting them', async () => {
    installStorage({
      [KEY]: JSON.stringify({
        version: 2,
        state: {
          hidden: [],
          sections: [{ id: 's1', columns: [['summary', 'netWorthChart']] }],
          options: { 'netWorthChart.view': 'removed-choice', 'oldCard.view': 'detailed', 'netWorthChart.gone': 'x' },
        },
      }),
    })
    const { useDashboardLayoutStore, optionValue } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    expect(useDashboardLayoutStore.getState().options).toEqual({})
    expect(optionValue(useDashboardLayoutStore.getState().options, 'netWorthChart', 'view')).toBe('simple')
  })

  it('keep a layout saved before options existed working', async () => {
    installStorage({
      [KEY]: JSON.stringify({ version: 2, state: { hidden: [], sections: [{ id: 's1', columns: [['summary']] }] } }),
    })
    const { useDashboardLayoutStore } = await freshStore()
    await useDashboardLayoutStore.persist.rehydrate()
    expect(useDashboardLayoutStore.getState().options).toEqual({})
  })

  it('are cleared by Reset along with the arrangement', async () => {
    const { useDashboardLayoutStore } = await freshStore()
    useDashboardLayoutStore.getState().setOption('netWorthChart', 'view', 'detailed')
    useDashboardLayoutStore.getState().reset()
    expect(useDashboardLayoutStore.getState().options).toEqual({})
  })
})
