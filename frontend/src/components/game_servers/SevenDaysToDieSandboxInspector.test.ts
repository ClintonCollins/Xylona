import { flushPromises, mount } from '@vue/test-utils'
import { Code, ConnectError } from '@connectrpc/connect'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieSandboxComparisonState,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import SevenDaysToDieSandboxInspector from './SevenDaysToDieSandboxInspector.vue'

const mocks = vi.hoisted(() => ({ getSettings: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getSevenDaysToDieSandboxSettings: mocks.getSettings }),
}))

const available =
  SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE
const valueAvailable =
  SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE

function mountInspector() {
  return mount(SevenDaysToDieSandboxInspector, {
    props: { gameServerId: 'server-12', refreshKey: 0 },
    global: {
      stubs: {
        'q-icon': { props: ['name'], template: '<i :data-icon="name" />' },
        'q-spinner': { template: '<i data-testid="spinner" />' },
      },
    },
  })
}

async function expandInspector() {
  const wrapper = mountInspector()
  await wrapper.get('.sandbox-summary').trigger('click')
  await flushPromises()
  return wrapper
}

function matchingResponse(code: string) {
  return {
    connectionState: available,
    state: valueAvailable,
    comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
    configuredCode: code,
    effectiveCode: code,
    settings: [
      {
        key: 'EnemySpawn',
        label: 'Enemy spawning',
        effectiveValue: code,
      },
    ],
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('SevenDaysToDieSandboxInspector', () => {
  beforeEach(() => {
    mocks.getSettings.mockReset()
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
      configuredCode: 'ABC',
      effectiveCode: 'ABC',
      settings: [],
    })
  })

  it('starts not checked without querying the game', () => {
    const wrapper = mountInspector()

    expect(wrapper.get('.sandbox-status').text()).toContain('Not checked')
    expect(wrapper.get('.sandbox-summary').attributes('aria-expanded')).toBe('false')
    expect(mocks.getSettings).not.toHaveBeenCalled()
  })

  it.each([
    {
      name: 'match',
      response: {
        connectionState: available,
        state: valueAvailable,
        comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
        settings: [],
      },
      text: 'Match',
    },
    {
      name: 'mismatch',
      response: {
        connectionState: available,
        state: valueAvailable,
        comparisonState: SevenDaysToDieSandboxComparisonState.MISMATCH,
        settings: [],
      },
      text: 'Mismatch',
    },
    {
      name: 'stale',
      response: {
        connectionState: available,
        state: valueAvailable,
        comparisonState: SevenDaysToDieSandboxComparisonState.STALE,
        settings: [],
      },
      text: 'Stale',
    },
    {
      name: 'offline',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
        comparisonState: SevenDaysToDieSandboxComparisonState.UNSPECIFIED,
        settings: [],
      },
      text: 'Offline',
    },
    {
      name: 'unsupported',
      response: {
        connectionState: available,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
        comparisonState: SevenDaysToDieSandboxComparisonState.UNSPECIFIED,
        settings: [],
      },
      text: 'Unsupported',
    },
    {
      name: 'native unauthorized',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
        comparisonState: SevenDaysToDieSandboxComparisonState.UNSPECIFIED,
        settings: [],
      },
      text: 'Unauthorized',
    },
    {
      name: 'unavailable',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
        comparisonState: SevenDaysToDieSandboxComparisonState.UNSPECIFIED,
        settings: [],
      },
      text: 'Unavailable',
    },
  ])('renders the $name state', async ({ response, text }) => {
    mocks.getSettings.mockResolvedValue(response)
    const wrapper = await expandInspector()

    expect(wrapper.get('.sandbox-status').text()).toContain(text)
  })

  it('shows the native empty state', async () => {
    const wrapper = await expandInspector()

    expect(wrapper.text()).toContain('The game reported no sandbox settings.')
  })

  it('routes by server id, groups and filters metadata, and renders upstream text as text', async () => {
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
      configuredCode: 'SAME',
      effectiveCode: 'SAME',
      settings: [
        {
          key: 'BloodMoonFrequency',
          label: '<script>Blood moon</script>',
          description: '<img src=x onerror=alert(1)>',
          group: 'World',
          effectiveValue: '3',
          effectiveLabel: 'Every 3 days',
        },
        {
          key: 'DayNightLength',
          label: 'Day length',
          description: 'Minutes per day',
          group: 'World',
          effectiveValue: '60',
        },
      ],
    })
    const wrapper = await expandInspector()

    expect(mocks.getSettings).toHaveBeenCalledWith(
      expect.objectContaining({ gameServerId: 'server-12' }),
    )
    expect(wrapper.text()).toContain('<script>Blood moon</script>')
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.find('.sandbox-inspector script').exists()).toBe(false)
    expect(wrapper.find('.sandbox-inspector img').exists()).toBe(false)
    expect(wrapper.text()).toContain('World')
    expect(wrapper.text()).toContain('Matches')

    await wrapper.get('input[type="search"]').setValue('day length')
    expect(wrapper.text()).not.toContain('<script>Blood moon</script>')
    expect(wrapper.text()).toContain('Day length')

    await wrapper.get('input[type="search"]').setValue('does not exist')
    expect(wrapper.text()).toContain('No settings match this filter.')
  })

  it('shows mismatched code settings as unpaired observations without a configured snapshot', async () => {
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.MISMATCH,
      configuredCode: 'SAVED',
      effectiveCode: 'RUNNING',
      settings: [
        {
          key: 'EnemySpawn',
          label: 'Enemy spawning',
          effectiveValue: 'true',
        },
      ],
    })
    const wrapper = await expandInspector()

    expect(wrapper.get('.sandbox-status').text()).toContain('Mismatch')
    expect(wrapper.text()).toContain('are observations, not per-setting comparisons')
    expect(wrapper.text()).toContain('Observed running')
    expect(wrapper.text()).toContain('Not compared')
    expect(wrapper.text()).not.toContain('Different')
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    expect(wrapper.findAll('th').map((header) => header.text())).not.toContain('Saved code')
  })

  it('renders loading, authorization, and failure without exposing errors', async () => {
    let rejectRequest: ((reason: unknown) => void) | undefined
    mocks.getSettings.mockReturnValue(
      new Promise((_, reject) => {
        rejectRequest = reject
      }),
    )
    const wrapper = mountInspector()
    await wrapper.get('.sandbox-summary').trigger('click')
    expect(wrapper.get('.sandbox-status').text()).toContain('Loading')

    rejectRequest?.(new ConnectError('secret upstream detail', Code.PermissionDenied))
    await flushPromises()
    expect(wrapper.get('.sandbox-status').text()).toContain('Unauthorized')
    expect(wrapper.text()).not.toContain('secret upstream detail')

    mocks.getSettings.mockRejectedValue(new Error('token=must-not-render'))
    await wrapper.get('.sandbox-refresh').trigger('click')
    await flushPromises()
    expect(wrapper.get('.sandbox-status').text()).toContain('Failed')
    expect(wrapper.text()).not.toContain('token=must-not-render')
  })

  it('renders backend stale observations without comparison claims', async () => {
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.STALE,
      configuredCode: 'INVALID',
      effectiveCode: 'RUNNING',
      settings: [
        {
          key: 'EnemySpawn',
          label: 'Enemy spawning',
          effectiveValue: 'true',
        },
      ],
    })
    const wrapper = await expandInspector()

    expect(wrapper.text()).toContain('running observations are not compared')
    expect(wrapper.text()).toContain('Observed running')
    expect(wrapper.text()).toContain('Not compared')
    expect(wrapper.text()).not.toContain('Different')
    expect(wrapper.text()).not.toContain('Matches')
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    expect(wrapper.findAll('th').map((header) => header.text())).not.toContain('Saved code')
  })

  it('marks old rows unpaired after SandboxCode is saved', async () => {
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
      configuredCode: 'OLD',
      effectiveCode: 'OLD',
      settings: [
        {
          key: 'EnemySpawn',
          label: 'Enemy spawning',
          effectiveValue: 'true',
        },
      ],
    })
    const wrapper = await expandInspector()
    await wrapper.setProps({ refreshKey: 1 })

    expect(wrapper.get('.sandbox-status').text()).toContain('Stale')
    expect(wrapper.text()).toContain('predate the current editor value')
    expect(wrapper.text()).toContain('Previously saved SandboxCode')
    expect(wrapper.text()).toContain('Previously observed running')
    expect(wrapper.text()).toContain('Not compared')
    expect(wrapper.text()).not.toContain('Matches')
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    expect(mocks.getSettings).toHaveBeenCalledOnce()
  })

  it('marks an initial response stale when SandboxCode is saved while it is pending', async () => {
    const pending = deferred<ReturnType<typeof matchingResponse>>()
    mocks.getSettings.mockReturnValue(pending.promise)
    const wrapper = mountInspector()

    await wrapper.get('.sandbox-summary').trigger('click')
    await wrapper.setProps({ refreshKey: 1 })
    pending.resolve(matchingResponse('OLD'))
    await flushPromises()

    expect(wrapper.get('.sandbox-status').text()).toContain('Stale')
    expect(wrapper.text()).toContain('predate the current editor value')
    expect(wrapper.text()).toContain('Previously saved SandboxCode')
    expect(wrapper.text()).toContain('Not compared')
    expect(wrapper.text()).not.toContain('Matches')
  })

  it('keeps old observations stale while a refresh is pending', async () => {
    mocks.getSettings.mockResolvedValueOnce(matchingResponse('OLD'))
    const wrapper = await expandInspector()
    await wrapper.setProps({ refreshKey: 1 })
    const pending = deferred<ReturnType<typeof matchingResponse>>()
    mocks.getSettings.mockReturnValueOnce(pending.promise)

    await wrapper.get('.sandbox-refresh').trigger('click')

    expect(wrapper.get('.sandbox-status').text()).toContain('Loading')
    expect(wrapper.text()).toContain('Previously saved SandboxCode')
    expect(wrapper.text()).toContain('Not compared')
    expect(wrapper.text()).not.toContain('Matches')

    pending.resolve(matchingResponse('NEW'))
    await flushPromises()

    expect(wrapper.get('.sandbox-status').text()).toContain('Match')
    expect(wrapper.text()).not.toContain('Previously saved SandboxCode')
    expect(wrapper.text()).toContain('NEW')
    expect(wrapper.text()).toContain('Matches')
  })

  it('ignores a superseded response that resolves last', async () => {
    const older = deferred<ReturnType<typeof matchingResponse>>()
    const newer = deferred<ReturnType<typeof matchingResponse>>()
    mocks.getSettings.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const wrapper = mountInspector()

    await wrapper.get('.sandbox-summary').trigger('click')
    const refresh = wrapper.get<HTMLButtonElement>('.sandbox-refresh')
    refresh.element.disabled = false
    await refresh.trigger('click')
    expect(mocks.getSettings).toHaveBeenCalledTimes(2)

    newer.resolve(matchingResponse('NEW'))
    await flushPromises()
    expect(wrapper.text()).toContain('NEW')

    older.resolve(matchingResponse('OLD'))
    await flushPromises()
    expect(wrapper.text()).toContain('NEW')
    expect(wrapper.text()).not.toContain('OLD')
  })
})
