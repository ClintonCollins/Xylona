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

  it('routes by server id, groups and filters metadata, and renders upstream text as text', async () => {
    mocks.getSettings.mockResolvedValue({
      connectionState: available,
      state: valueAvailable,
      comparisonState: SevenDaysToDieSandboxComparisonState.MISMATCH,
      configuredCode: 'SAVED',
      effectiveCode: 'RUNNING',
      settings: [
        {
          key: 'BloodMoonFrequency',
          label: '<script>Blood moon</script>',
          description: '<img src=x onerror=alert(1)>',
          group: 'World',
          configuredValue: '7',
          configuredLabel: 'Every 7 days',
          effectiveValue: '3',
          effectiveLabel: 'Every 3 days',
          matches: false,
        },
        {
          key: 'DayNightLength',
          label: 'Day length',
          description: 'Minutes per day',
          group: 'World',
          configuredValue: '60',
          effectiveValue: '60',
          matches: true,
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
    expect(wrapper.text()).toContain('Different')

    await wrapper.get('input[type="search"]').setValue('day length')
    expect(wrapper.text()).not.toContain('<script>Blood moon</script>')
    expect(wrapper.text()).toContain('Day length')
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

  it('marks an observed comparison stale after SandboxCode is saved', async () => {
    const wrapper = await expandInspector()
    await wrapper.setProps({ refreshKey: 1 })

    expect(wrapper.get('.sandbox-status').text()).toContain('Stale')
    expect(mocks.getSettings).toHaveBeenCalledOnce()
  })
})
