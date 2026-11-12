import { describe, expect, it } from 'vitest'
import { plaidSyncProblem } from './plaidStatus'

describe('plaidSyncProblem', () => {
  it('is null when the connection is syncing normally', () => {
    expect(plaidSyncProblem({ status: 'active' })).toBeNull()
    // A stale message on a healthy connection is not a problem.
    expect(plaidSyncProblem({ status: 'active', lastSyncError: 'old failure' })).toBeNull()
  })

  it('is null when the connection is not known (still loading, or not Plaid)', () => {
    expect(plaidSyncProblem(undefined)).toBeNull()
  })

  it('asks the user to reconnect when the bank needs a new sign-in', () => {
    expect(plaidSyncProblem({ status: 'login_required' })).toMatch(/sign in again/i)
  })

  it('reports the failure message when a sync errors', () => {
    expect(plaidSyncProblem({ status: 'error', lastSyncError: 'liabilities: rate limited' })).toBe(
      'The last sync failed: liabilities: rate limited',
    )
  })

  it('still flags an error that has no message', () => {
    expect(plaidSyncProblem({ status: 'error' })).toMatch(/last sync failed/i)
  })
})
