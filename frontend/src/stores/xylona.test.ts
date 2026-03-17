import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CheckUserAuthenticatedResponseSchema,
  UserSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from './xylona'

const mocks = vi.hoisted(() => ({
  checkUserAuthenticated: vi.fn(),
  logout: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    checkUserAuthenticated: mocks.checkUserAuthenticated,
    logout: mocks.logout,
  }),
  ConnectErrorToString: (err: ConnectError) => err.message,
}))

vi.mock('quasar', () => ({
  Notify: {
    create: vi.fn(),
  },
}))

describe('useUserAuthStore — checkUserAuthenticated', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.checkUserAuthenticated.mockReset()
    mocks.logout.mockReset()
  })

  it('success: sets user, initialFetch=true, returns response', async () => {
    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'admin',
      email: 'admin@example.com',
      firstName: 'Admin',
      lastName: 'User',
      superUser: true,
    })

    const response = create(CheckUserAuthenticatedResponseSchema, {
      authenticated: true,
      user,
    })

    mocks.checkUserAuthenticated.mockResolvedValueOnce(response)

    const store = useUserAuthStore()
    const result = await store.checkUserAuthenticated()

    expect(mocks.checkUserAuthenticated).toHaveBeenCalledTimes(1)
    expect(store.initialFetch).toBe(true)
    expect(store.user).toMatchObject({
      id: 'user-1',
      userName: 'admin',
    })
    expect(result).toStrictEqual(response)
    expect(store.initialResponse).toStrictEqual(response)
  })

  it('failure: resets initialFetch to false and shows Notify error', async () => {
    const { Notify } = await import('quasar')
    const error = new ConnectError('connection refused')
    mocks.checkUserAuthenticated.mockRejectedValueOnce(error)

    const store = useUserAuthStore()
    const result = await store.checkUserAuthenticated()

    expect(result).toBeNull()
    expect(store.initialFetch).toBe(false)
    expect(store.user).toBeNull()
    expect(Notify.create).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
        position: 'top',
      }),
    )
  })

  it('caching: second call returns cached response without API call', async () => {
    const user = create(UserSchema, {
      id: 'user-2',
      userName: 'cached-user',
    })

    const response = create(CheckUserAuthenticatedResponseSchema, {
      authenticated: true,
      user,
    })

    mocks.checkUserAuthenticated.mockResolvedValueOnce(response)

    const store = useUserAuthStore()

    const firstResult = await store.checkUserAuthenticated()
    expect(firstResult).toStrictEqual(response)
    expect(mocks.checkUserAuthenticated).toHaveBeenCalledTimes(1)

    const secondResult = await store.checkUserAuthenticated()
    expect(secondResult).toStrictEqual(response)
    // Should not have called the API again
    expect(mocks.checkUserAuthenticated).toHaveBeenCalledTimes(1)
  })
})

describe('useUserAuthStore — logout', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.checkUserAuthenticated.mockReset()
    mocks.logout.mockReset()
  })

  it('success: clears user, initialFetch, and initialResponse', async () => {
    const store = useUserAuthStore()
    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'admin',
    })

    store.setUser(user)
    store.initialFetch = true
    store.initialResponse = create(CheckUserAuthenticatedResponseSchema, {
      authenticated: true,
      user,
    })

    mocks.logout.mockResolvedValueOnce({})

    await store.logout()

    expect(mocks.logout).toHaveBeenCalledTimes(1)
    expect(store.user).toBeNull()
    expect(store.initialFetch).toBe(false)
    expect(store.initialResponse).toBeNull()
  })

  it('failure: logs error and does not crash', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const error = new ConnectError('logout failed')
    mocks.logout.mockRejectedValueOnce(error)

    const store = useUserAuthStore()
    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'admin',
    })
    store.setUser(user)

    // Should not throw
    await store.logout()

    expect(consoleSpy).toHaveBeenCalledWith(
      'Logout error:',
      expect.any(String),
    )

    // User should NOT be cleared since the API call failed
    expect(store.user).not.toBeNull()

    consoleSpy.mockRestore()
  })
})
