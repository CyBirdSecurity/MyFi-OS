import { Component, type ReactNode } from 'react'
import { Button } from '../ui/Button'

/**
 * Catches a crash inside a page so the rest of the app (navigation, other
 * pages) keeps working and the user sees a message instead of a blank screen.
 * AppShell keys it by route, so navigating away resets it.
 */
export class PageErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: unknown) {
    console.error('Page crashed:', error)
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div className="max-w-md mx-auto mt-16 bg-surface border border-border-default rounded-lg p-6 text-center">
        <h2 className="text-lg font-semibold text-text-primary">Something went wrong on this page</h2>
        <p className="mt-2 text-sm text-text-secondary">
          The rest of MyFi still works. Try reloading, or use the menu to go to another page.
        </p>
        <Button className="mt-5" onClick={() => window.location.reload()}>Reload page</Button>
      </div>
    )
  }
}
