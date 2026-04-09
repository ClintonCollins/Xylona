import { createPinia, setActivePinia } from 'pinia'
import { create as createProto } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import { NodeSchema } from '@/proto/shared_pb'
import { UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import GameServerList from './GameServerList.vue'

const mocks = vi.hoisted(() => ({
  listAggregatedGameServers: vi.fn(),
  listNodes: vi.fn(),
  registerServerContext: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listAggregatedGameServers: mocks.listAggregatedGameServers,
      listNodes: mocks.listNodes,
      startGameServer: vi.fn(),
      stopGameServer: vi.fn(),
    }),
    XylonaEventBus: {
      on: vi.fn(),
    },
  }
})

vi.mock('@/utils/game-server-notifications', () => ({
  registerServerContext: mocks.registerServerContext,
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
      screen: { lt: { md: false } },
    }),
  }
})

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useStorage: <T>(_: string, initialValue: T) => ref(initialValue),
  }
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<button>{{ label }}</button>',
})

describe('GameServerList', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.listAggregatedGameServers.mockReset()
    mocks.listNodes.mockReset()
    mocks.registerServerContext.mockReset()
    mocks.listAggregatedGameServers.mockResolvedValue({ servers: [] })
    mocks.listNodes.mockResolvedValue({
      nodes: [
        createProto(NodeSchema, {
          id: 'node-local',
          name: 'Local Node',
          host: 'localhost',
          port: 8080n,
          local: true,
          secretKey: '',
          baseUrl: 'http://localhost:8080',
          enabled: true,
          lastSyncStatus: '',
          healthStatus: '',
          version: '',
          protocolVersion: 0n,
          capabilities: '',
          allowInsecureTls: false,
          departed: false,
          autoPaired: false,
        }),
      ],
    })
  })

  function mountList(superUser: boolean) {
    const store = useUserAuthStore()
    store.user = createProto(UserSchema, {
      id: superUser ? 'user-admin' : 'user-owner',
      userName: superUser ? 'admin' : 'owner',
      superUser,
    })

    return mount(GameServerList, {
      global: {
        stubs: {
          'q-page': { template: '<div><slot /></div>' },
          'q-input': { template: '<div><slot /></div>' },
          'q-icon': true,
          'q-badge': true,
          'q-td': { template: '<div><slot /></div>' },
          'q-tooltip': true,
          'q-table': { template: '<div><slot /><slot name="no-data" /></div>' },
          'router-link': { template: '<a><slot /></a>' },
          'delete-game-server-dialog': true,
          StatusBadge: true,
          VersionBadge: true,
          'q-btn': QBtnStub,
        },
      },
    })
  }

  it('hides the create button for non-superusers', async () => {
    const wrapper = mountList(false)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Create Game Server')
  })

  it('shows the create button for superusers', async () => {
    const wrapper = mountList(true)
    await flushPromises()

    expect(wrapper.text()).toContain('Create Game Server')
  })
})
