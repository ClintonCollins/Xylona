import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi, beforeEach } from 'vitest'

import { GameSchema, GameServerSchema, IPSchema, NodeSchema } from '@/proto/shared_pb'
import GameServerSettingsForm from './GameServerSettingsForm.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  listGames: vi.fn(),
  listNodes: vi.fn(),
  listUsers: vi.fn(),
  listIPs: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getGameServer: mocks.getGameServer,
      listGames: mocks.listGames,
      listNodes: mocks.listNodes,
      listUsers: mocks.listUsers,
      listIPs: mocks.listIPs,
      editGameServer: vi.fn(),
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      back: vi.fn(),
      push: vi.fn(),
    }),
  }
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-input-stub" v-bind="$attrs">{{ label }}</div>',
})

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-select-stub" v-bind="$attrs">{{ label }}</div>',
})

function mountSettingsForm(canEditProvisioning: boolean) {
  return mount(GameServerSettingsForm, {
    props: {
      gameServerId: 'server-local-1',
      canEditProvisioning,
    },
    global: {
      stubs: {
        'q-form': { template: '<form><slot /></form>' },
        'q-input': QInputStub,
        'q-select': QSelectStub,
        'q-btn': { template: '<button><slot />{{ label }}</button>', props: ['label'] },
        'q-icon': true,
        'q-spinner-dots': true,
        'q-inner-loading': true,
        'router-link': { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('GameServerSettingsForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        disconnect() {}
        observe() {}
      },
    )

    mocks.getGameServer.mockReset()
    mocks.listGames.mockReset()
    mocks.listNodes.mockReset()
    mocks.listUsers.mockReset()
    mocks.listIPs.mockReset()

    mocks.listGames.mockResolvedValue({
      games: [
        create(GameSchema, {
          id: 'minecraft',
          name: 'Minecraft',
          defaultPort: 25565n,
          defaultQueryPort: 25565n,
          defaultMaxPlayers: 20n,
        }),
      ],
    })
    mocks.listNodes.mockResolvedValue({
      nodes: [
        create(NodeSchema, {
          id: 'node-local',
          name: 'Local Node',
          local: true,
        }),
      ],
    })
    mocks.listUsers.mockResolvedValue({
      users: [{ id: 'user-owner', userName: 'owner' }],
    })
    mocks.listIPs.mockResolvedValue({
      ips: [
        create(IPSchema, {
          address: '127.0.0.1',
        }),
      ],
    })
  })

  it('shows full editable provisioning controls for superusers', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 25565n,
        queryPort: 25565n,
        setMaxPlayers: 20n,
        maxPlayers: 20n,
        maxMemoryMb: 1024n,
        startCommand: 'java -jar server.jar',
      }),
    })

    const wrapper = mountSettingsForm(true)
    await flushPromises()

    expect(wrapper.find('[data-testid="editable-game"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-owner"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-node"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-ip"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-port"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-query-port"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-max-players"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-max-memory"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="readonly-provisioning"]').exists()).toBe(false)
  })

  it('shows read-only provisioning context and only editable operational fields for non-superusers', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 25565n,
        queryPort: 25565n,
        setMaxPlayers: 20n,
        maxPlayers: 20n,
        maxMemoryMb: 1024n,
        startCommand: 'java -jar server.jar',
      }),
    })

    const wrapper = mountSettingsForm(false)
    await flushPromises()

    expect(wrapper.find('[data-testid="readonly-provisioning"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="readonly-game"]').text()).toContain('Minecraft')
    expect(wrapper.find('[data-testid="readonly-owner"]').text()).toContain('owner')
    expect(wrapper.find('[data-testid="readonly-node"]').text()).toContain('Local Node')
    expect(wrapper.find('[data-testid="readonly-connection"]').text()).toContain('127.0.0.1:25565')
    expect(wrapper.find('[data-testid="readonly-capacity"]').text()).toContain('20')
    expect(wrapper.find('[data-testid="readonly-max-memory"]').text()).toContain('1024')

    expect(wrapper.find('[data-testid="editable-name"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-set-players"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-start-command"]').exists()).toBe(true)

    expect(wrapper.find('[data-testid="editable-game"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-owner"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-node"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-ip"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-port"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-query-port"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-max-players"]').exists()).toBe(false)
  })

  it('hides minecraft memory context when the server is not minecraft', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: create(GameServerSchema, {
        id: 'server-local-1',
        name: 'TF2 Server',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'team-fortress-2',
        gameName: 'Team Fortress 2',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 27015n,
        queryPort: 27016n,
        setMaxPlayers: 24n,
        maxPlayers: 24n,
        maxMemoryMb: 0n,
        startCommand: './srcds_run',
      }),
    })

    const wrapper = mountSettingsForm(false)
    await flushPromises()

    expect(wrapper.find('[data-testid="readonly-max-memory"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-max-memory"]').exists()).toBe(false)
  })
})
