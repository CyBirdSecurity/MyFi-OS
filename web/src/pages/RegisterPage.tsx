import { useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import useSWR, { mutate } from 'swr'
import { authApi, configApi } from '../api/client'
import { useAuthStore } from '../store/authStore'
import { Button } from '../components/ui/Button'

const inputCls =
  'w-full bg-surface border border-border-default rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:ring-2 focus:ring-accent'
const labelCls = 'block text-sm text-text-secondary mb-1'

// First-run setup: creates the instance's only account. Once it exists this
// page sends visitors to sign in.
export function RegisterPage() {
  const navigate = useNavigate()
  const setUser = useAuthStore((s) => s.setUser)
  const [displayName, setDisplayName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const { data: config } = useSWR('/config', () => configApi.get(), {
    revalidateOnFocus: false,
  })
  if (!config) return null
  if (!config.setupRequired) return <Navigate to="/login" replace />

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await authApi.register(email, password, displayName)
      // The cached /config still says setup is required; refresh it.
      await mutate('/config')
      const user = await authApi.login(email, password)
      setUser(user)
      navigate('/onboarding')
    } catch (err: any) {
      setError(err.message ?? 'Registration failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-canvas flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-accent-text">MyFi</h1>
          <p className="text-text-tertiary mt-2 text-sm">Your personal finance dashboard</p>
        </div>
        <form
          onSubmit={handleSubmit}
          className="bg-surface border border-border-default rounded-lg p-6 space-y-4"
        >
          <h2 className="text-lg font-semibold text-text-primary">Set up MyFi</h2>
          <p className="text-sm text-text-tertiary">Create the account for this MyFi instance. It's the only one, so keep the password somewhere safe.</p>
          {error && (
            <div className="text-negative text-sm bg-negative-tint-bg rounded-lg px-3 py-2">
              {error}
            </div>
          )}
          <div>
            <label className={labelCls}>Name</label>
            <input type="text" value={displayName} onChange={(e) => setDisplayName(e.target.value)} required className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Email</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Password</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} className={inputCls} />
          </div>
          <Button type="submit" loading={loading} className="w-full justify-center">
            Create account
          </Button>
        </form>
      </div>
    </div>
  )
}
