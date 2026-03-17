import { create } from '@bufbuild/protobuf'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UserSchema } from '@/proto/xylona_pb'
import { useToolbarNavQTabsStore, useUserAuthStore } from './xylona'

const mocks = vi.hoisted(() => ({
  logout: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    logout: mocks.logout,
  }),
  ConnectErrorToString: (err: Error) => err.message,
}))

describe('useUserAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.logout.mockReset()
  })

  it('initializes with empty auth state', () => {
    const store = useUserAuthStore()
    expect(store.user).toBeNull()
    expect(store.initialFetch).toBe(false)
    expect(store.initialResponse).toBeNull()
  })

  it('sets authenticated user data', () => {
    const store = useUserAuthStore()
    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'admin',
      email: 'admin@example.com',
      firstName: 'Admin',
      lastName: 'User',
      superUser: true,
    })

    store.setUser(user)
    expect(store.user).toMatchObject({
      id: 'user-1',
      userName: 'admin',
      email: 'admin@example.com',
      superUser: true,
    })
  })

  it('resets state on logout', async () => {
    const store = useUserAuthStore()
    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'admin',
    })

    store.setUser(user)
    store.initialFetch = true
    store.initialResponse = { user, authenticated: true } as any

    mocks.logout.mockResolvedValueOnce({})

    await store.logout()

    expect(mocks.logout).toHaveBeenCalledTimes(1)
    expect(store.user).toBeNull()
    expect(store.initialFetch).toBe(false)
    expect(store.initialResponse).toBeNull()
  })
})

describe('useToolbarNavQTabsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('initializes with empty tabs', () => {
    const store = useToolbarNavQTabsStore()
    expect(store.selectedTab).toBe('')
    expect(store.tabs).toEqual([])
  })

  it('replaces tabs with the provided configuration', () => {
    const store = useToolbarNavQTabsStore()
    store.changeTabs([
      { name: 'Home', to: '/', exact: true, icon: 'home' },
      { name: 'Servers', to: '/servers', exact: false, icon: 'dns' },
    ])

    expect(store.tabs).toEqual([
      { name: 'Home', to: '/', exact: true, icon: 'home' },
      { name: 'Servers', to: '/servers', exact: false, icon: 'dns' },
    ])
  })
})
