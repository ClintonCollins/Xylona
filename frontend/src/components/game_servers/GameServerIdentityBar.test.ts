import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GameServerSchema, Status } from '@/proto/shared_pb'
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'

import GameServerIdentityBar from './GameServerIdentityBar.vue'

const mocks = vi.hoisted(() => ({
  dialogChoice: { value: 'ok' as 'ok' | 'dismiss' },
  dialog: vi.fn(),
  emit: vi.fn(),
  getGameServerReadiness: vi.fn(),
  notifyConnectError: vi.fn(),
  playerCount: 0 as number | null,
  push: vi.fn(),
  restartGameServer: vi.fn(),
  startGameServer: vi.fn(),
  stopGameServer: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ dialog: mocks.dialog, screen: { lt: { sm: false } } }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/game-servers/server-1/files' }),
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock('@/api/notifications', () => ({ notifyConnectError: mocks.notifyConnectError }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerReadiness: mocks.getGameServerReadiness,
    restartGameServer: mocks.restartGameServer,
    startGameServer: mocks.startGameServer,
    stopGameServer: mocks.stopGameServer,
  }),
  XylonaEventBus: { emit: mocks.emit, off: vi.fn(), on: vi.fn() },
}))

vi.mock('@/pages/game_servers/useGameServerQueryStatusVersion', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return {
    useGameServerQueryStatusVersion: () => ({
      currentPlayerCount: ref(mocks.playerCount ?? 0),
      playerCount: ref(mocks.playerCount),
      maxPlayerCount: ref(20),
      onlinePlayers: ref([]),
      playerListSupported: ref(true),
      unknownPlayersMessage: ref('Player count and names unavailable.'),
      queryFresh: ref(true),
      queryGameServer: vi.fn(),
      startQueryStatusVersionLifecycle: vi.fn(),
    }),
  }
})

function mountBar(status: Status, effectivePermissions: string[] = []) {
  return shallowMount(GameServerIdentityBar, {
    props: {
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        name: 'Survival',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        status,
        effectivePermissions,
      }),
    },
    global: { renderStubDefaultSlot: true },
  })
}

function button(wrapper: ReturnType<typeof mountBar>, label: string) {
  const found = wrapper.findAll('q-btn-stub').find((btn) => btn.attributes('label') === label)
  if (found === undefined) throw new Error(`no ${label} button`)
  return found
}

describe('GameServerIdentityBar', () => {
  beforeEach(() => {
    setWebsocketConnectionStatus('connected')
    mocks.playerCount = 0
    mocks.dialogChoice.value = 'ok'
    mocks.dialog.mockImplementation(() => {
      const chain = {
        onOk(callback: () => void) {
          if (mocks.dialogChoice.value === 'ok') callback()
          return chain
        },
        onDismiss(callback: () => void) {
          callback()
          return chain
        },
      }
      return chain
    })
    mocks.getGameServerReadiness.mockResolvedValue({ items: [] })
    mocks.startGameServer.mockResolvedValue({})
    mocks.stopGameServer.mockResolvedValue({})
    mocks.restartGameServer.mockResolvedValue({})
  })

  afterEach(() => {
    setWebsocketConnectionStatus('connecting')
    vi.clearAllMocks()
  })

  it.each([
    { status: Status.OFFLINE, start: true, restart: false, stop: false },
    { status: Status.ONLINE, start: false, restart: true, stop: true },
    { status: Status.PRE_START, start: false, restart: true, stop: true },
    { status: Status.UNKNOWN, start: false, restart: false, stop: false },
  ])('enables only the lifecycle actions status $status allows', ({ status, ...want }) => {
    const wrapper = mountBar(status)

    expect(button(wrapper, 'Start').attributes('disable')).toBe(want.start ? 'false' : 'true')
    expect(button(wrapper, 'Restart').attributes('disable')).toBe(want.restart ? 'false' : 'true')
    expect(button(wrapper, 'Stop').attributes('disable')).toBe(want.stop ? 'false' : 'true')
  })

  it('keeps an action disabled without its permission', () => {
    const wrapper = mountBar(Status.ONLINE, ['game_server.stop'])

    expect(button(wrapper, 'Stop').attributes('disable')).toBe('false')
    expect(button(wrapper, 'Restart').attributes('disable')).toBe('true')
    expect(button(wrapper, 'Restart').attributes('aria-label')).toBe(
      'Restart (requires restart permission)',
    )
  })

  it.each([
    {
      label: 'confirms a stop that disconnects players',
      action: 'Stop',
      playerCount: 3,
      dialogChoice: 'ok' as const,
      wantDialog: { title: 'Stop Survival?', label: 'Stop server' },
      wantCalls: 1,
    },
    {
      label: 'keeps the server running when the stop confirm is cancelled',
      action: 'Stop',
      playerCount: 3,
      dialogChoice: 'dismiss' as const,
      wantDialog: { title: 'Stop Survival?', label: 'Stop server' },
      wantCalls: 0,
    },
    {
      label: 'confirms a restart that disconnects players',
      action: 'Restart',
      playerCount: 1,
      dialogChoice: 'ok' as const,
      wantDialog: { title: 'Restart Survival?', label: 'Restart server' },
      wantCalls: 1,
    },
    {
      label: 'confirms a stop when the player count is unknown',
      action: 'Stop',
      playerCount: null,
      dialogChoice: 'ok' as const,
      wantDialog: { title: 'Stop Survival?', label: 'Stop server' },
      wantCalls: 1,
    },
    {
      label: 'restarts at once when nobody is online',
      action: 'Restart',
      playerCount: 0,
      dialogChoice: 'ok' as const,
      wantDialog: null,
      wantCalls: 1,
    },
  ])('$label', async ({ action, playerCount, dialogChoice, wantDialog, wantCalls }) => {
    mocks.playerCount = playerCount
    mocks.dialogChoice.value = dialogChoice
    const wrapper = mountBar(Status.ONLINE)

    await button(wrapper, action).trigger('click')
    await flushPromises()

    if (wantDialog === null) {
      expect(mocks.dialog).not.toHaveBeenCalled()
    } else {
      expect(mocks.dialog).toHaveBeenCalledWith(
        expect.objectContaining({
          title: wantDialog.title,
          ok: expect.objectContaining({ label: wantDialog.label }),
        }),
      )
    }
    const rpc = action === 'Stop' ? mocks.stopGameServer : mocks.restartGameServer
    expect(rpc).toHaveBeenCalledTimes(wantCalls)
    if (wantCalls > 0)
      expect(rpc).toHaveBeenCalledWith(expect.objectContaining({ serverId: 'server-1' }))
  })

  it('shows a rejected start and asks the console to re-read readiness', async () => {
    mocks.startGameServer.mockRejectedValue(
      new ConnectError('accept the EULA first', Code.FailedPrecondition),
    )
    const wrapper = mountBar(Status.OFFLINE)

    await button(wrapper, 'Start').trigger('click')
    await flushPromises()

    expect(wrapper.get('.start-failure').text()).toContain('accept the EULA first')
    expect(mocks.emit).toHaveBeenCalledWith('gameServerStartRejected', 'server-1')
    expect(button(wrapper, 'Show output').attributes('to')).toBe('/game-servers/server-1/console')
  })

  it('disables Start and names the setup step that blocks it', async () => {
    mocks.getGameServerReadiness.mockResolvedValue({
      items: [
        {
          kind: 'dragonwilds_config',
          required: true,
          blocking: true,
          complete: false,
          message: 'Set the Owner ID.',
        },
      ],
    })
    const wrapper = mountBar(Status.OFFLINE)
    await flushPromises()

    const start = button(wrapper, 'Start')
    expect(start.attributes('disable')).toBe('true')
    expect(start.attributes('aria-label')).toBe(
      'Start (blocked until Dragonwilds configuration is finished)',
    )
    // The hint sits beside the button, since a disabled button gets no hover.
    expect(start.element.parentElement?.textContent).toContain(
      'Dragonwilds configuration: Set the Owner ID.',
    )
    // Touch screens get no tooltip, so the reason and the fix sit under the bar too.
    expect(wrapper.get('.identity-bar-blocker').text()).toContain('Set the Owner ID.')
    expect(button(wrapper, 'Open Configuration').attributes('to')).toBe(
      '/game-servers/server-1/configuration',
    )
  })
})
