import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Question } from '@phosphor-icons/react'
import { authApi } from '../api/client'
import { useAuthStore } from '../store/authStore'
import { usePlaidConfig } from '../hooks/usePlaid'
import { Modal } from './ui/Modal'
import { Button } from './ui/Button'

// IDs (and 'getting-started') must match pageGuides in internal/api/onboarding_handler.go.
export type GuideId =
  | 'dashboard'
  | 'accounts'
  | 'holdings'
  | 'forecast'
  | 'retirement'
  | 'scenarios'
  | 'goals'
  | 'budget'
  | 'taxes'
  | 'import'

// A point that reads differently depending on whether Plaid is configured;
// `withoutPlaid` omitted means the point is hidden when Plaid is off.
type Point = string | { withPlaid: string; withoutPlaid?: string }

interface Guide {
  title: string
  intro: string
  points: Point[]
  action?: { label: string; to: string }
}

const GUIDES: Record<GuideId, Guide> = {
  dashboard: {
    title: 'Dashboard',
    intro: 'A snapshot of your whole financial picture: your net worth, how it has changed, where your money sits, what you spend, and your goals.',
    points: [
      'Net worth is everything you own minus everything you owe. It updates as your balances change.',
      'Everything here is built from your accounts, so the more you add, the more complete the picture.',
      'A card with nothing in it yet links to the step that fills it.',
      'Use Customize to show, hide, reorder, and resize cards.',
    ],
    action: { label: 'Add an account', to: '/accounts' },
  },
  accounts: {
    title: 'Accounts',
    intro: 'Every account you own or owe on: checking, savings, brokerage, retirement, crypto, property, loans, and credit cards.',
    points: [
      { withPlaid: 'Connect Institution links a bank or brokerage through Plaid so balances and transactions update automatically.' },
      {
        withPlaid: '+ Add Account lets you enter any account by hand, including ones Plaid can’t reach, like a home or a car.',
        withoutPlaid: '+ Add Account adds any account, including a home or a car. Update balances by hand or with a file import.',
      },
      'Loans and credit cards count as liabilities and are subtracted from your net worth.',
      'Open an account to update its balance, add holdings, or view its history.',
    ],
  },
  holdings: {
    title: 'Holdings',
    intro: 'Every investment you own across all accounts, combined and priced live.',
    points: [
      'See your cost basis, unrealized gains, and how concentrated you are in any single position.',
      'Set allocation targets in Settings to see how far your portfolio has drifted from them.',
      'Click a ticker to see its price history and which accounts hold it.',
    ],
  },
  forecast: {
    title: 'Forecast',
    intro: 'Projects the savings and investments you can retire on, year by year through age 100, under base, optimistic, and pessimistic scenarios.',
    points: [
      'The projection uses your current balances, contributions, expected return rates, and retirement age.',
      'Add contributions on each account so the forecast includes your ongoing saving.',
      "Homes, cars, and other property aren't counted, since you can't spend them down. Debts are paid down using the rate and payment on each card or loan.",
      'Adjust return rates and inflation in Settings if you prefer different assumptions.',
    ],
  },
  retirement: {
    title: 'Retirement',
    intro: 'Shows how long your portfolio lasts once you stop working and start withdrawing from it.',
    points: [
      'Your FI Number is the portfolio size that can cover your retirement spending indefinitely.',
      'Your safe withdrawal rate is the share of your portfolio you take out each year. 4% is the common rule of thumb.',
      'Set your retirement spending and withdrawal rate in Settings to sharpen the results.',
    ],
  },
  scenarios: {
    title: 'Scenarios',
    intro: 'Test changes to your plan before you make them.',
    points: [
      'What-If compares up to 3 alternatives side by side with your current plan, such as retiring earlier or saving more.',
      'Solve For works backwards: pick a target and one variable, and MyFi finds the value that gets you there.',
    ],
  },
  goals: {
    title: 'Goals',
    intro: 'Set milestones and track your progress toward them automatically.',
    points: [
      'Track a goal against your total net worth, a single account, an account type, or an asset class.',
      'Spending goals help you save for a purchase, like a car or a down payment.',
      'Your top active goals also appear on the dashboard.',
    ],
  },
  budget: {
    title: 'Budget',
    intro: 'Where your money goes each month.',
    points: [
      'Overview breaks down your spending by category, Income tracks what comes in, and Transactions lists everything.',
      { withPlaid: 'Transactions come from Plaid, file imports, or the + button.', withoutPlaid: 'Transactions come from file imports or the + button.' },
      'Set up bills that repeat, like a mortgage, once on the Recurring tab and they’re added automatically.',
      'Imported transactions MyFi isn’t sure about are flagged for you to review and categorize.',
    ],
  },
  taxes: {
    title: 'Tax Planning',
    intro: 'Plan around taxes now and in retirement.',
    points: [
      'See where your income falls in the tax brackets, and plan Roth conversions and withdrawal order.',
      'Track ACA health-insurance subsidy limits if you retire before Medicare.',
      'Set your filing status, state, and dependents in Settings for accurate numbers.',
    ],
  },
  import: {
    title: 'Import & Export',
    intro: 'Bring in data from other places, or take yours with you.',
    points: [
      'Upload CSV or OFX files from your broker or bank to add holdings and transactions.',
      'Export a full backup of your MyFi data at any time.',
    ],
  },
}

/**
 * Whether a one-time prompt should still show for the current user, and a
 * function that hides it for good (saved on the server).
 */
export function useGuideDismissal(id: GuideId | 'getting-started') {
  const { user, setUser } = useAuthStore()
  const pending = !!user && !user.dismissedGuides?.includes(id)
  const dismiss = () => {
    if (!pending || !user) return
    setUser({ ...user, dismissedGuides: [...(user.dismissedGuides ?? []), id] })
    authApi.dismissGuide(id).catch(() => {})
  }
  return { pending, dismiss }
}

/**
 * The "?" button for a page header. Opens the page's guide automatically on
 * the user's first visit, and on demand after that.
 */
export function PageGuide({ id }: { id: GuideId }) {
  const navigate = useNavigate()
  const { pending, dismiss } = useGuideDismissal(id)
  const [open, setOpen] = useState(pending)
  const guide = GUIDES[id]
  const { data: config } = usePlaidConfig()
  const points = guide.points.flatMap((p) => {
    if (typeof p === 'string') return [p]
    const text = config?.plaidEnabled ? p.withPlaid : p.withoutPlaid
    return text ? [text] : []
  })

  const close = () => {
    setOpen(false)
    dismiss()
  }

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title="About this page"
        aria-label="About this page"
        className="text-text-muted hover:text-accent-text transition-colors"
      >
        <Question size={20} weight="bold" />
      </button>
      <Modal open={open} onClose={close} title={guide.title}>
        <p className="text-sm text-text-secondary">{guide.intro}</p>
        <ul className="mt-4 space-y-2 text-sm text-text-primary list-disc pl-5">
          {points.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
        <div className="mt-6 flex justify-end gap-3">
          {guide.action ? (
            <>
              <Button variant="secondary" onClick={close}>Maybe later</Button>
              <Button onClick={() => { close(); navigate(guide.action!.to) }}>{guide.action.label}</Button>
            </>
          ) : (
            <Button onClick={close}>Got it</Button>
          )}
        </div>
      </Modal>
    </>
  )
}
