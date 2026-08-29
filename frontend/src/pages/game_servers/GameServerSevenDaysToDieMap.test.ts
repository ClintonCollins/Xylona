import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieMapViewSchema,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIStatusSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import GameServerSevenDaysToDieMap from './GameServerSevenDaysToDieMap.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  getMap: vi.fn(),
  getStatus: vi.fn(),
  installLandClaims: vi.fn(),
  notifyConnectError: vi.fn(),
  notifySuccess: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    getSevenDaysToDieMap: mocks.getMap,
    getSevenDaysToDieWebAPIStatus: mocks.getStatus,
    installSevenDaysToDieLandClaimsMod: mocks.installLandClaims,
  }),
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notifyConnectError,
  notifySuccess: mocks.notifySuccess,
}))

const availableStatus = create(SevenDaysToDieWebAPIStatusSchema, {
  connectionState:
    SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
  apiVersion: 'V2.2',
  capabilities: {
    playerData: true,
    runtimeSettings: true,
    nativeLog: true,
    worldPopulation: true,
    hostileAndAnimalPositions: false,
    hostilePositions: true,
    animalPositions: false,
    accessControl: false,
    gamePermissions: true,
    reportedMods: true,
  },
  worldTimeState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  worldTime: { day: 42, hour: 13, minute: 7 },
  bloodMoonState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  bloodMoonActive: false,
  nextBloodMoon: { day: 49, hour: 22 },
  nextBloodMoonEnd: { day: 50, hour: 4, minute: 30 },
})

const worldMap = create(SevenDaysToDieMapViewSchema, {
  enabled: true,
  tileSize: 128,
  maxZoom: 4,
  mapSize: { x: 6144, z: 6144 },
  players: [
    { id: 'player-1', name: 'Alex', online: true, position: { x: 1, z: 2 } },
    { id: 'player-2', name: 'Blake', position: { x: 3, z: 4 } },
  ],
  markers: [
    { id: 'marker-1', name: 'Trader', x: 10, z: 20 },
    { id: 'marker-2', name: 'Base', x: 30, z: 40 },
  ],
  claimsState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  claims: [{ ownerName: 'Owner', size: 41, position: { x: 30, z: 40 } }],
  hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  hostiles: [{ name: 'Zombie', position: { x: 50, z: 60 } }],
  animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  animals: [
    { name: 'Wolf', position: { x: 70, z: 80 } },
    { name: 'Deer', position: { x: 90, z: 100 } },
  ],
})

function mountPage() {
  return shallowMount(GameServerSevenDaysToDieMap, {
    global: {
      renderStubDefaultSlot: true,
      stubs: { SevenDaysToDieWorldOverview: false },
    },
  })
}

describe('GameServerSevenDaysToDieMap world overview', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-22T12:00:20Z'))
    mocks.getGameServer.mockReset().mockResolvedValue({ gameServer: null })
    mocks.getMap.mockReset().mockResolvedValue({ map: worldMap })
    mocks.getStatus.mockReset().mockResolvedValue({ status: availableStatus })
    mocks.installLandClaims.mockReset().mockResolvedValue({})
    mocks.notifyConnectError.mockReset()
    mocks.notifySuccess.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('puts the map first and renders existing world data in a compact overview', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: {
        effectivePermissions: ['game_server.view', 'game_server.settings', 'game_server.config'],
      },
    })
    let resolveStatus: ((value: { status: typeof availableStatus }) => void) | undefined
    mocks.getStatus.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveStatus = resolve
      }),
    )

    const wrapper = mountPage()
    expect(wrapper.text()).toContain('Loading world data')

    resolveStatus?.({ status: availableStatus })
    await flushPromises()

    const overview = wrapper.get('[data-testid="world-overview"]')
    expect(overview.attributes('aria-live')).toBe('polite')
    expect(overview.text()).toContain('Day 42, 13:07')
    expect(overview.text()).toContain('Inactive')
    expect(overview.text()).toContain('Day 49, 22:00')
    expect(overview.text()).toContain('Day 50, 04:30')
    expect(overview.text()).toContain('1 online · 2 known')
    expect(overview.text()).toContain('6,144 × 6,144')
    expect(overview.text()).toContain('Map notes2')
    expect(overview.text()).toContain('Land claims1')
    expect(overview.text()).toContain('Hostiles1')
    expect(overview.text()).toContain('Animals2')
    expect(wrapper.text()).not.toContain('WebAPI diagnostics')
    expect(wrapper.text()).not.toContain('API V2.2')
    expect(wrapper.text()).not.toContain('Capability details')
    expect(wrapper.find('[data-testid="install-land-claim-helper"]').exists()).toBe(true)

    const map = wrapper.getComponent(SevenDaysToDieLiveMap)
    expect(map.props('configurationPath')).toBe('/game-servers/server-1/configuration')
    expect(
      map.element.compareDocumentPosition(overview.element) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    wrapper.unmount()
  })

  it.each([
    {
      name: 'offline',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE,
      text: 'Server offline',
    },
    {
      name: 'disabled',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED,
      text: 'Web dashboard disabled',
    },
    {
      name: 'unsupported',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED,
      text: 'API discovery unsupported',
    },
    {
      name: 'denied',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED,
      text: 'WebAPI access denied',
    },
    {
      name: 'unavailable',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE,
      text: 'Node unavailable',
    },
    {
      name: 'invalid',
      state:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE,
      text: 'Invalid WebAPI response',
    },
  ])('renders the $name connection state', async ({ state, text }) => {
    mocks.getStatus.mockResolvedValue({
      status: create(SevenDaysToDieWebAPIStatusSchema, { connectionState: state }),
    })

    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain(text)
    wrapper.unmount()
  })

  it.each([
    {
      name: 'unsupported',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
      text: 'Not supported by this WebAPI',
    },
    {
      name: 'denied',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
      text: 'Access denied by the game server',
    },
    {
      name: 'unavailable',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      text: 'Unavailable',
    },
  ])('renders $name Blood Moon access independently', async ({ state, text }) => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    mocks.getStatus.mockResolvedValue({
      status: create(SevenDaysToDieWebAPIStatusSchema, {
        ...availableStatus,
        bloodMoonState: state,
        bloodMoonActive: undefined,
        nextBloodMoon: undefined,
        nextBloodMoonEnd: undefined,
      }),
    })

    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Day 42, 13:07')
    expect(wrapper.get('[data-testid="blood-moon-state"]').text()).toContain(text)
    wrapper.unmount()
  })

  it('retains the last available details and marks them stale after a later failure', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getStatus.mockResolvedValueOnce({ status: availableStatus })
    mocks.getStatus.mockRejectedValueOnce(new Error('node disconnected'))

    const wrapper = mountPage()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()

    expect(errorSpy).toHaveBeenCalled()
    expect(wrapper.text()).toContain('World data unavailable')
    expect(wrapper.text()).toContain('Day 42, 13:07')
    expect(wrapper.text()).not.toContain('Inactive')
    expect(wrapper.text()).not.toContain('Day 49, 22:00')
    expect(wrapper.text()).not.toContain('Day 50, 04:30')
    wrapper.unmount()
  })

  it('shows a typed failure while retaining stale details', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    mocks.getStatus.mockResolvedValueOnce({ status: availableStatus }).mockResolvedValueOnce({
      status: create(SevenDaysToDieWebAPIStatusSchema, {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE,
      }),
    })

    const wrapper = mountPage()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()

    expect(wrapper.text()).toContain('Server offline')
    expect(wrapper.text()).toContain('Day 42, 13:07')
    expect(wrapper.text()).not.toContain('Inactive')
    expect(wrapper.text()).not.toContain('Day 49, 22:00')
    expect(wrapper.find('[data-testid="install-land-claim-helper"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('installs unsupported land claim support once and explains the required start', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    mocks.getMap.mockResolvedValue({
      map: create(SevenDaysToDieMapViewSchema, {
        ...worldMap,
        claims: [],
        claimsState:
          SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
      }),
    })
    let resolveInstall: ((value: object) => void) | undefined
    mocks.installLandClaims.mockReturnValue(
      new Promise((resolve) => {
        resolveInstall = resolve
      }),
    )

    const wrapper = mountPage()
    await flushPromises()

    const installButton = wrapper.get('[data-testid="install-land-claim-helper"]')
    expect(installButton.text()).toContain('Install / repair claims')
    await installButton.trigger('click')
    await installButton.trigger('click')

    expect(mocks.installLandClaims).toHaveBeenCalledTimes(1)
    expect(mocks.installLandClaims.mock.calls[0]?.[0].gameServerId).toBe('server-1')

    resolveInstall?.({})
    await flushPromises()

    expect(mocks.notifySuccess).toHaveBeenCalledWith(
      'Land claim support installed. It will load the next time the server starts.',
    )
    expect(wrapper.find('[data-testid="install-land-claim-helper"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([
    {
      name: 'unspecified',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED,
    },
    {
      name: 'permission-denied',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
    },
    {
      name: 'unavailable',
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
    },
  ])('keeps manual installation available for the $name state', async ({ state }) => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    mocks.getMap.mockResolvedValue({
      map: create(SevenDaysToDieMapViewSchema, {
        ...worldMap,
        claims: [],
        claimsState: state,
      }),
    })

    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.find('[data-testid="install-land-claim-helper"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps the repair action available when installation fails', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view', 'game_server.settings'] },
    })
    mocks.getMap.mockResolvedValue({
      map: create(SevenDaysToDieMapViewSchema, {
        ...worldMap,
        claims: [],
        claimsState:
          SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
      }),
    })
    const installError = new Error('write failed')
    mocks.installLandClaims.mockRejectedValue(installError)

    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-testid="install-land-claim-helper"]').trigger('click')
    await flushPromises()

    expect(mocks.notifyConnectError).toHaveBeenCalledWith(
      installError,
      'Failed to install land claim support',
    )
    expect(wrapper.find('[data-testid="install-land-claim-helper"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows base world stats while never displaying tactical data to a viewer', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { effectivePermissions: ['game_server.view'] },
    })
    mocks.getStatus.mockResolvedValue({ status: availableStatus })

    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Day 42, 13:07')
    expect(wrapper.text()).toContain('1 online · 2 known')
    expect(wrapper.text()).toContain('6,144 × 6,144')
    expect(wrapper.find('[data-testid="blood-moon-state"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Land claims')
    expect(wrapper.text()).not.toContain('Hostiles')
    expect(wrapper.text()).not.toContain('Animals')
    expect(wrapper.text()).not.toContain('Inactive')
    expect(wrapper.text()).not.toContain('Day 49, 22:00')
    wrapper.unmount()
  })

  it('refreshes map and overview together and stops both polling loops on unmount', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getStatus.mockRejectedValueOnce(new Error('temporary failure'))
    mocks.getStatus.mockResolvedValue({ status: availableStatus })

    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('World data unavailable')

    let resolveMap: ((value: { map: typeof worldMap }) => void) | undefined
    let resolveStatus: ((value: { status: typeof availableStatus }) => void) | undefined
    mocks.getMap.mockReturnValueOnce(new Promise((resolve) => (resolveMap = resolve)))
    mocks.getStatus.mockReturnValueOnce(new Promise((resolve) => (resolveStatus = resolve)))

    const map = wrapper.getComponent(SevenDaysToDieLiveMap)
    map.vm.$emit('refresh')
    await Promise.resolve()
    expect(map.props('refreshing')).toBe(true)

    resolveMap?.({ map: worldMap })
    resolveStatus?.({ status: availableStatus })
    await flushPromises()
    expect(map.props('refreshing')).toBe(false)
    expect(wrapper.text()).not.toContain('World data unavailable')
    expect(errorSpy).toHaveBeenCalledTimes(1)
    expect(mocks.getStatus).toHaveBeenCalledTimes(2)
    expect(mocks.getMap).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(mocks.getStatus).toHaveBeenCalledTimes(3)
    expect(mocks.getMap).toHaveBeenCalledTimes(8)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(mocks.getStatus).toHaveBeenCalledTimes(3)
    expect(mocks.getMap).toHaveBeenCalledTimes(8)
  })

  it('clears tactical data immediately after a rejected map poll while retaining base data', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const tacticalMap = create(SevenDaysToDieMapViewSchema, {
      enabled: true,
      tileSize: 128,
      maxZoom: 4,
      mapSize: { x: 6144, z: 6144 },
      players: [{ id: 'player-1', name: 'Alex', online: true, position: { x: 1, z: 2 } }],
      nativeMarkerState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      nativeMarkers: [{ id: 'marker-1', name: 'Trader', x: 10, z: 20 }],
      claimsState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      claims: [{ ownerName: 'Owner', size: 41, position: { x: 30, z: 40 } }],
      bloodMoonState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      bloodMoon: {
        gameTime: { day: 7, hour: 22 },
        nextBloodMoon: { day: 14, hour: 22 },
        nextBloodMoonEnd: { day: 15, hour: 4 },
      },
      hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostiles: [{ name: 'Zombie', position: { x: 50, z: 60 } }],
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animals: [{ name: 'Wolf', position: { x: 70, z: 80 } }],
    })
    mocks.getMap
      .mockResolvedValueOnce({ map: tacticalMap })
      .mockRejectedValueOnce(new Error('node disconnected'))

    const wrapper = mountPage()
    await flushPromises()
    let mapProps = wrapper.getComponent(SevenDaysToDieLiveMap).props('view')
    expect(mapProps.players).toHaveLength(1)
    expect(mapProps.hostiles).toHaveLength(1)

    await vi.advanceTimersByTimeAsync(5_000)
    await flushPromises()
    mapProps = wrapper.getComponent(SevenDaysToDieLiveMap).props('view')
    const unavailable =
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
    expect(mapProps.players).toHaveLength(1)
    expect(mapProps.nativeMarkers).toHaveLength(0)
    expect(mapProps.claims).toHaveLength(0)
    expect(mapProps.bloodMoon).toBeUndefined()
    expect(mapProps.hostiles).toHaveLength(0)
    expect(mapProps.animals).toHaveLength(0)
    expect([
      mapProps.nativeMarkerState,
      mapProps.claimsState,
      mapProps.bloodMoonState,
      mapProps.hostileState,
      mapProps.animalState,
    ]).toEqual([unavailable, unavailable, unavailable, unavailable, unavailable])
    expect(errorSpy).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
