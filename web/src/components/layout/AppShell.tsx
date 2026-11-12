import { Suspense } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { SideNav } from './SideNav'
import { BottomNav } from './BottomNav'
import { PageErrorBoundary } from './PageErrorBoundary'

export function AppShell() {
  const { pathname } = useLocation()
  return (
    <div className="flex min-h-screen bg-canvas">
      <SideNav />
      <main className="flex-1 lg:ml-56 pb-[calc(6rem+env(safe-area-inset-bottom))] lg:pb-6 overflow-y-auto min-w-0">
        <div className="p-4 sm:p-6">
          <PageErrorBoundary key={pathname}>
            <Suspense fallback={null}>
              <Outlet />
            </Suspense>
          </PageErrorBoundary>
        </div>
      </main>
      <BottomNav />
    </div>
  )
}
