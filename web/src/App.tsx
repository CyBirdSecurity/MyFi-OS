import { lazy, Suspense, useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import useSWR from 'swr'
import { authApi, configApi } from './api/client'
import { useAuthStore } from './store/authStore'
import { LoginPage } from './pages/LoginPage'

// Everything except the public entry pages loads on demand, so visitors don't
// download the whole app (charts included) before the first screen.
const AppShell = lazyPage(() => import('./components/layout/AppShell'), 'AppShell')
const RegisterPage = lazyPage(() => import('./pages/RegisterPage'), 'RegisterPage')
const DashboardPage = lazyPage(() => import('./pages/DashboardPage'), 'DashboardPage')
const AccountsPage = lazyPage(() => import('./pages/AccountsPage'), 'AccountsPage')
const AccountDetailPage = lazyPage(() => import('./pages/AccountDetailPage'), 'AccountDetailPage')
const HoldingsPage = lazyPage(() => import('./pages/HoldingsPage'), 'HoldingsPage')
const GoalsPage = lazyPage(() => import('./pages/GoalsPage'), 'GoalsPage')
const ForecastPage = lazyPage(() => import('./pages/ForecastPage'), 'ForecastPage')
const SettingsPage = lazyPage(() => import('./pages/SettingsPage'), 'SettingsPage')
const TickerDetailPage = lazyPage(() => import('./pages/TickerDetailPage'), 'TickerDetailPage')
const OFXImportPage = lazyPage(() => import('./pages/OFXImportPage'), 'OFXImportPage')
const ImportPage = lazyPage(() => import('./pages/ImportPage'), 'ImportPage')
const BudgetPage = lazyPage(() => import('./pages/BudgetPage'), 'BudgetPage')
const TaxesPage = lazyPage(() => import('./pages/TaxesPage'), 'TaxesPage')
const OnboardingPage = lazyPage(() => import('./pages/OnboardingPage'), 'OnboardingPage')
const RetirementPage = lazyPage(() => import('./pages/RetirementPage'), 'RetirementPage')
const ScenariosPage = lazyPage(() => import('./pages/ScenariosPage'), 'ScenariosPage')

// Loads a named page export lazily. After a deploy, an open tab may ask for a
// chunk that no longer exists; reload once to pick up the new build.
function lazyPage<K extends string>(
  load: () => Promise<Record<K, React.ComponentType>>,
  name: K,
) {
  return lazy(() =>
    load().then(
      (m) => {
        sessionStorage.removeItem('chunk-reload')
        return { default: m[name] }
      },
      (err) => {
        if (!sessionStorage.getItem('chunk-reload')) {
          sessionStorage.setItem('chunk-reload', '1')
          window.location.reload()
          return new Promise<never>(() => {})
        }
        throw err
      },
    ),
  )
}

function RequireAuth({
  children,
  authChecked,
}: {
  children: React.ReactNode
  authChecked: boolean
}) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (!authChecked) return null
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

// New users see the welcome screen before the app.
function RequireOnboarded({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user)
  if (user && !user.onboardedAt) return <Navigate to="/onboarding" replace />
  return <>{children}</>
}

// "/" sends a fresh instance to first-run setup, then to sign-in or the app.
function RootRedirect({ authChecked }: { authChecked: boolean }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const { data: config } = useSWR('/config', () => configApi.get(), { revalidateOnFocus: false })
  if (!authChecked) return null
  if (isAuthenticated) return <Navigate to="/dashboard" replace />
  if (!config) return null
  return <Navigate to={config.setupRequired ? '/register' : '/login'} replace />
}

export default function App() {
  const { setUser } = useAuthStore()
  const [authChecked, setAuthChecked] = useState(false)

  useEffect(() => {
    authApi
      .me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setAuthChecked(true))
  }, [])

  return (
    <BrowserRouter>
      <Suspense fallback={null}>
        <Routes>
          <Route path="/" element={<RootRedirect authChecked={authChecked} />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />

          <Route
            path="/onboarding"
            element={
              <RequireAuth authChecked={authChecked}>
                <OnboardingPage />
              </RequireAuth>
            }
          />

          <Route
            element={
              <RequireAuth authChecked={authChecked}>
                <RequireOnboarded>
                  <AppShell />
                </RequireOnboarded>
              </RequireAuth>
            }
          >
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/accounts" element={<AccountsPage />} />
            <Route path="/accounts/:id" element={<AccountDetailPage />} />
            <Route path="/holdings" element={<HoldingsPage />} />
            <Route path="/holdings/:ticker" element={<TickerDetailPage />} />
            <Route path="/forecast" element={<ForecastPage />} />
            <Route path="/retirement" element={<RetirementPage />} />
            <Route path="/scenarios" element={<ScenariosPage />} />
            <Route path="/goals" element={<GoalsPage />} />
            <Route path="/budget" element={<BudgetPage />} />
            <Route path="/taxes" element={<TaxesPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/import" element={<ImportPage />} />
            <Route path="/import/ofx" element={<OFXImportPage />} />
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  )
}
