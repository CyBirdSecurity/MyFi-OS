import { useState } from 'react'
import { CalendarBlank } from '@phosphor-icons/react'
import { Modal } from './ui/Modal'
import { Button } from './ui/Button'
import { BirthdaySetupForm } from './SetupForms'
import { PageGuide, type GuideId } from './PageGuide'

/**
 * Shown in place of a projection page (Forecast, Retirement) until the user
 * has entered their date of birth, which every projection depends on.
 */
export function ProfileRequired({
  title,
  guideId,
  subtitle,
  what,
}: {
  title: string
  guideId: GuideId
  subtitle: string
  /** What the page will show once set up, e.g. "your net worth projected to age 100". */
  what: string
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="space-y-6 max-w-6xl">
      <div>
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold text-text-primary">{title}</h1>
          <PageGuide id={guideId} />
        </div>
        <p className="text-sm text-text-tertiary mt-1">{subtitle}</p>
      </div>
      <div className="bg-surface border border-border-default rounded-lg px-6 py-12 flex flex-col items-center text-center">
        <div className="mb-4 inline-flex rounded-lg bg-accent-tint-bg p-3 ring-1 ring-accent-tint-border">
          <CalendarBlank size={24} weight="bold" className="text-accent-text" />
        </div>
        <h2 className="text-lg font-semibold text-text-primary">Add your date of birth to get started</h2>
        <p className="mt-2 max-w-md text-sm text-text-secondary">
          MyFi needs your age and target retirement age to show {what}.
        </p>
        <Button className="mt-6" onClick={() => setOpen(true)}>Add date of birth</Button>
      </div>
      <Modal open={open} onClose={() => setOpen(false)} title="Your Details">
        <BirthdaySetupForm onDone={() => setOpen(false)} />
      </Modal>
    </div>
  )
}
