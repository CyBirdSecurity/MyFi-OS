import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import {
  appendWidget,
  fromOrder,
  moveWidget,
  newSectionId,
  normalize,
  removeWidget,
  setWidth,
  type DropTarget,
  type Width,
  type Section,
} from '../components/Dashboard/layoutModel'

export type DashboardWidgetId =
  | 'summary'
  | 'netWorthChart'
  | 'spendingSummary'
  | 'recentTransactions'
  | 'allocation'
  | 'goals'
  | 'topMovers'
  | 'debtPayoff'
  | 'mortgagePayoff'
  | 'household'
  | 'readiness'
  | 'taxBreakdown'
  | 'cashFlow'
  | 'portfolioCheckup'

/** A choice a card offers in the editor, such as the net worth chart's Simple or Detailed view. */
export interface WidgetOption {
  key: string
  label: string
  choices: { value: string; label: string }[]
  default: string
}

export const DASHBOARD_WIDGETS: {
  id: DashboardWidgetId
  label: string
  size: 'full' | 'half'
  options?: WidgetOption[]
}[] = [
  { id: 'summary', label: 'Net Worth', size: 'full' },
  {
    id: 'netWorthChart',
    label: 'Net Worth Over Time',
    size: 'full',
    options: [
      {
        key: 'view',
        label: 'View',
        choices: [
          { value: 'simple', label: 'Simple' },
          { value: 'detailed', label: 'Detailed' },
        ],
        default: 'simple',
      },
    ],
  },
  { id: 'spendingSummary', label: "This Month's Spending", size: 'half' },
  { id: 'recentTransactions', label: 'Recent Transactions', size: 'half' },
  { id: 'allocation', label: 'Allocation', size: 'half' },
  { id: 'goals', label: 'Goals', size: 'half' },
  { id: 'topMovers', label: 'Top Movers', size: 'half' },
  { id: 'debtPayoff', label: 'Debt Payoff Progress', size: 'half' },
  { id: 'mortgagePayoff', label: 'Mortgage Payoff', size: 'half' },
  { id: 'household', label: 'Household Breakdown', size: 'full' },
  { id: 'readiness', label: 'Retirement Readiness', size: 'full' },
  { id: 'taxBreakdown', label: 'Tax Treatment Breakdown', size: 'full' },
  { id: 'cashFlow', label: 'Cash Flow & Savings Rate', size: 'half' },
  { id: 'portfolioCheckup', label: 'Portfolio Check-up', size: 'half' },
]

const DEFAULT_ORDER: DashboardWidgetId[] = DASHBOARD_WIDGETS.map((w) => w.id)

const optionDef = (id: string, key: string) =>
  DASHBOARD_WIDGETS.find((w) => w.id === id)?.options?.find((o) => o.key === key)

/** The chosen value of a card's option, or its default when none (or an unknown one) is saved. */
export function optionValue(options: Record<string, string>, id: DashboardWidgetId, key: string): string {
  const def = optionDef(id, key)
  if (!def) return ''
  const saved = options[`${id}.${key}`]
  return def.choices.some((c) => c.value === saved) ? saved : def.default
}

/** Keeps only saved options that still name a real card option and one of its choices. */
function cleanOptions(raw: unknown): Record<string, string> {
  const clean: Record<string, string> = {}
  if (!raw || typeof raw !== 'object') return clean
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    const [id, key] = k.split('.')
    if (typeof v === 'string' && optionDef(id, key)?.choices.some((c) => c.value === v)) clean[k] = v
  }
  return clean
}
const SIZE_BY_ID = new Map(DASHBOARD_WIDGETS.map((w) => [w.id as string, w.size]))
const isHalf = (id: string) => SIZE_BY_ID.get(id) === 'half'

/**
 * The out-of-the-box arrangement, as rows of columns of stacked cards. A row
 * with one column is full width; with two, the columns sit side by side and a
 * column can stack several cards. Cards a user has nothing to show for (no
 * partner, no debts) simply don't render, and their space goes to the rest.
 */
const DEFAULT_LAYOUT: DashboardWidgetId[][][] = [
  [['summary']],
  [['netWorthChart']],
  [['spendingSummary'], ['goals']],
  [['allocation'], ['household', 'readiness']],
  [['recentTransactions'], ['debtPayoff', 'mortgagePayoff']],
  [['taxBreakdown']],
]

/**
 * Cards that start hidden; they're one click away under Add card. They are also
 * optional for people who already have a saved layout: a card added to the app
 * later (like Cash Flow) is offered in Add card rather than appearing unasked.
 */
const DEFAULT_HIDDEN: DashboardWidgetId[] = ['topMovers', 'cashFlow', 'portfolioCheckup']

/** `hidden`, plus every optional card that `seen` (the cards a saved layout knows about) lacks. */
function hideUnseenOptional(seen: Iterable<string>, hidden: DashboardWidgetId[]): DashboardWidgetId[] {
  const known = new Set(seen)
  const extra = DEFAULT_HIDDEN.filter((id) => !known.has(id) && !hidden.includes(id))
  return [...hidden, ...extra]
}

function defaultSections(): Section[] {
  return DEFAULT_LAYOUT.map((columns) => ({
    id: newSectionId(),
    columns: columns.map((col) => [...col]),
  }))
}

interface DashboardLayoutState {
  /** Rows of the dashboard; each has one or two columns of stacked widgets. */
  sections: Section[]
  hidden: DashboardWidgetId[]
  /** Chosen card options, keyed "<cardId>.<optionKey>"; read with optionValue(). */
  options: Record<string, string>
  moveWidget: (id: DashboardWidgetId, target: DropTarget) => void
  setWidth: (id: DashboardWidgetId, width: Width) => void
  setOption: (id: DashboardWidgetId, key: string, value: string) => void
  hideWidget: (id: DashboardWidgetId) => void
  showWidget: (id: DashboardWidgetId) => void
  reset: () => void
}

export const useDashboardLayoutStore = create<DashboardLayoutState>()(
  persist(
    (set, get) => ({
      sections: defaultSections(),
      hidden: [...DEFAULT_HIDDEN],
      options: {},
      moveWidget: (id, target) => set({ sections: moveWidget(get().sections, id, target) }),
      setWidth: (id, width) => set({ sections: setWidth(get().sections, id, width) }),
      setOption: (id, key, value) => {
        if (!optionDef(id, key)?.choices.some((c) => c.value === value)) return
        set({ options: { ...get().options, [`${id}.${key}`]: value } })
      },
      hideWidget: (id) => {
        const { hidden, sections } = get()
        if (hidden.includes(id)) return
        set({ hidden: [...hidden, id], sections: removeWidget(sections, id) })
      },
      showWidget: (id) => {
        const { hidden, sections } = get()
        if (!hidden.includes(id)) return
        set({ hidden: hidden.filter((h) => h !== id), sections: appendWidget(sections, id) })
      },
      reset: () => set({ sections: defaultSections(), hidden: [...DEFAULT_HIDDEN], options: {} }),
    }),
    {
      name: 'myfi-dashboard-layout',
      version: 2,
      // v0/v1 saved a flat `order` plus `hidden`. Rebuild the rows from it so
      // nobody's saved arrangement is lost.
      migrate: (persisted) => {
        const old = (persisted ?? {}) as { order?: string[]; hidden?: DashboardWidgetId[] }
        const hidden = hideUnseenOptional(old.order ?? [], old.hidden ?? [])
        const order = [...(old.order ?? []), ...DEFAULT_ORDER.filter((id) => !(old.order ?? []).includes(id))]
        return { hidden, sections: fromOrder(order, isHalf, hidden) } as DashboardLayoutState
      },
      // Reconcile a saved layout against the current widget list: drop ids that
      // no longer exist and add widgets that postdate the saved layout.
      merge: (persisted, current) => {
        const saved = (persisted ?? {}) as Partial<DashboardLayoutState>
        // Nothing saved yet (a new user) keeps the defaults, hidden cards included.
        let hidden = (saved.hidden ?? current.hidden).filter((id) => DEFAULT_ORDER.includes(id))
        // A saved layout predates any optional card it doesn't mention; keep those hidden.
        if (saved.sections) hidden = hideUnseenOptional(saved.sections.flatMap((s) => s.columns.flat()), hidden)
        return {
          ...current,
          hidden,
          sections: normalize(saved.sections ?? current.sections, DEFAULT_ORDER, hidden),
          options: cleanOptions(saved.options),
        }
      },
    },
  ),
)

/** The current value of a card's option, re-rendering when it changes. */
export function useWidgetOption(id: DashboardWidgetId, key: string): string {
  return useDashboardLayoutStore((s) => optionValue(s.options, id, key))
}
