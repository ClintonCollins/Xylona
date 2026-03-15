import { create } from '@bufbuild/protobuf'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { UserSchema } from '@/proto/xylona_pb'
import { useToolbarNavQTabsStore, useUserAuthStore } from './xylona'

describe('useUserAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
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
