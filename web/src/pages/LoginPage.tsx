import { useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import useSWR from 'swr'
import { Eye } from '@phosphor-icons/react'
import { authApi, configApi } from '../api/client'
import { useAuthStore } from '../store/authStore'
import { Button } from '../components/ui/Button'

const DEMO_EMAIL = 'demo@myfi.local'
const DEMO_PASSWORD = 'demo1234'

const inputCls =
  'w-full bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

export function LoginPage() {
  const navigate = useNavigate()
  const setUser = useAuthStore((s) => s.setUser)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [demoLoading, setDemoLoading] = useState(false)

  const { data: config } = useSWR('/config', () => configApi.get(), {
    revalidateOnFocus: false,
  })
  const isDev = config?.env === 'development'
  if (config?.setupRequired) return <Navigate to="/register" replace />

  const doLogin = async (loginEmail: string, loginPassword: string) => {
    const user = await authApi.login(loginEmail, loginPassword)
    setUser(user)
    navigate('/dashboard')
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await doLogin(email, password)
    } catch (err: any) {
      setError(err.message ?? 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  const handleDemoLogin = async () => {
    setError('')
    setDemoLoading(true)
    try {
      await doLogin(DEMO_EMAIL, DEMO_PASSWORD)
    } catch {
      setError('Demo account unavailable. Run `make seed` to create it.')
    } finally {
      setDemoLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-canvas flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <div className="flex justify-center mb-3">
            <img src="/logo-icon.png" alt="MyFi" className="h-14 w-14 object-contain" />
          </div>
          <h1 className="text-3xl font-bold text-accent-text">MyFi</h1>
          <p className="text-text-tertiary mt-2 text-sm">Your personal finance dashboard</p>
        </div>
        <form
          onSubmit={handleSubmit}
          className="bg-surface border border-border-default rounded-lg p-6 space-y-4"
        >
          <h2 className="text-lg font-semibold text-text-primary">Sign in</h2>
          {error && (
            <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-3 py-2">
              {error}
            </div>
          )}
          <div>
            <label className={labelCls}>Email</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Password</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required className={inputCls} />
          </div>
          <Button type="submit" loading={loading} className="w-full justify-center">Sign in</Button>
          <p className="text-xs text-text-muted">
            Forgot your password? Reset it on the server with{' '}
            <code className="font-mono text-text-secondary">myfi reset-password</code>. See the README.
          </p>
        </form>

        {isDev && (
          <div className="mt-4 bg-warning-tint-bg border border-warning/30 rounded-lg p-4">
            <p className="text-xs text-warning mb-3 text-center">Development environment</p>
            <button
              type="button"
              onClick={handleDemoLogin}
              disabled={demoLoading}
              className="w-full flex items-center justify-center gap-2 rounded-lg border border-warning/40 px-4 py-2.5 text-sm font-medium text-warning transition-colors hover:bg-warning/10 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {demoLoading ? (
                <span className="w-4 h-4 border-2 border-warning border-t-transparent rounded-full animate-spin" />
              ) : (
                <Eye size={16} weight="bold" />
              )}
              Check out a test environment
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
