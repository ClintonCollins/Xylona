import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIStatusSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import GameServerSevenDaysToDieMap from './GameServerSevenDaysToDieMap.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  getMap: vi.fn(),
  getStatus: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    getSevenDaysToDieMap: mocks.getMap,
    getSevenDaysToDieWebAPIStatus: mocks.getStatus,
  }),
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
  observedAt: timestampFromDate(new Date('2026-08-22T12:00:00Z')),
})

function mountPage() {
  return shallowMount(GameServerSevenDaysToDieMap, {
    global: { renderStubDefaultSlot: true },
  })
}

describe('GameServerSevenDaysToDieMap diagnostics', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-22T12:00:20Z'))
    mocks.getGameServer.mockReset().mockResolvedValue({ gameServer: null })
    mocks.getMap.mockReset().mockResolvedValue({ map: null })
    mocks.getStatus.mockReset().mockResolvedValue({ status: availableStatus })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('loads immediately and renders available diagnostics as text', async () => {
    let resolveStatus: ((value: { status: typeof availableStatus }) => void) | undefined
    mocks.getStatus.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveStatus = resolve
      }),
    )

    const wrapper = mountPage()
    expect(wrapper.text()).toContain('Checking WebAPI diagnostics')

    resolveStatus?.({ status: availableStatus })
    await flushPromises()

    expect(wrapper.text()).toContain('WebAPI available')
    expect(wrapper.text()).toContain('API V2.2')
    expect(wrapper.text()).toContain('Day 42, 13:07')
    expect(wrapper.text()).toContain('Inactive')
    expect(wrapper.text()).toContain('Day 49, 22:00')
    expect(wrapper.text()).toContain('Day 50, 04:30')
    expect(wrapper.text()).toContain('Observed 20 seconds ago')
    expect(wrapper.get('[data-testid="webapi-diagnostics"]').attributes('aria-live')).toBe('polite')

    const details = wrapper.get('[data-testid="webapi-capabilities"]')
    expect(details.attributes('open')).toBeUndefined()
    await details.get('summary').trigger('click')
    expect(details.attributes('open')).toBeDefined()
    expect(details.text()).toContain('Player data')
    expect(details.text()).toContain('Hostile positions')
    expect(details.text()).toContain('Animal positions')
    expect(details.text()).toContain('Supported')
    expect(details.text()).toContain('Not advertised')
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
    expect(wrapper.find('[data-testid="webapi-capabilities"]').exists()).toBe(false)
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
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getStatus.mockResolvedValueOnce({ status: availableStatus })
    mocks.getStatus.mockRejectedValueOnce(new Error('node disconnected'))

    const wrapper = mountPage()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()

    expect(errorSpy).toHaveBeenCalled()
    expect(wrapper.text()).toContain('Diagnostics unavailable')
    expect(wrapper.text()).toContain('Last successful observation')
    expect(wrapper.text()).toContain('API V2.2')
    expect(wrapper.text()).toContain('Day 42, 13:07')
    wrapper.unmount()
  })

  it('shows a typed failure while retaining stale details', async () => {
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
    expect(wrapper.text()).toContain('Last successful observation')
    expect(wrapper.text()).toContain('API V2.2')
    wrapper.unmount()
  })

  it('retries a failed request and stops both polling loops on unmount', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getStatus.mockRejectedValueOnce(new Error('temporary failure'))
    mocks.getStatus.mockResolvedValue({ status: availableStatus })

    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('Diagnostics unavailable')

    await wrapper.get('[data-testid="webapi-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('WebAPI available')
    expect(errorSpy).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(mocks.getStatus).toHaveBeenCalledTimes(3)
    expect(mocks.getMap).toHaveBeenCalledTimes(7)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(mocks.getStatus).toHaveBeenCalledTimes(3)
    expect(mocks.getMap).toHaveBeenCalledTimes(7)
  })
})
