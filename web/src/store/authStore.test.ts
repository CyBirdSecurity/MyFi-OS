import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../api/client'

const mutate = vi.hoisted(() => vi.fn())
vi.mock('swr', () => ({ mutate }))

import { useAuthStore } from './authStore'

const user = (id: string) => ({ id, email: `${id}@example.test`, displayName: id }) as User

describe('authStore.setUser', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, isAuthenticated: false })
    mutate.mockClear()
  })

  it('keeps the cache on the first sign-in after page load', () => {
    useAuthStore.getState().setUser(user('a'))
    expect(mutate).not.toHaveBeenCalled()
  })

  it('keeps the cache when the same user is updated', () => {
    useAuthStore.getState().setUser(user('a'))
    useAuthStore.getState().setUser({ ...user('a'), displayName: 'renamed' })
    expect(mutate).not.toHaveBeenCalled()
  })

  it('clears the cache on sign-out', () => {
    useAuthStore.getState().setUser(user('a'))
    useAuthStore.getState().setUser(null)
    expect(mutate).toHaveBeenCalledTimes(1)
    expect(mutate.mock.calls[0][1]).toBeUndefined()
    expect(mutate.mock.calls[0][2]).toEqual({ revalidate: false })
  })

  it('clears the cache when a different user takes over the tab', () => {
    useAuthStore.getState().setUser(user('a'))
    useAuthStore.getState().setUser(user('b'))
    expect(mutate).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().user?.id).toBe('b')
  })
})
