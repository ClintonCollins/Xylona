import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Status } from '@/proto/shared_pb'
import { GameServerDiagnosisSchema } from '@/proto/xylona_pb'
import GameServerDiagnosisPanel from './GameServerDiagnosisPanel.vue'

const mocks = vi.hoisted(() => ({ getGameServerDiagnosis: vi.fn() }))
vi.mock('@/utils/shared', () => ({ GetXylonaClient: () => mocks }))

function report() {
  return create(GameServerDiagnosisSchema, {
    executionId: 'attempt-1',
    category: 'port_in_use',
    stage: 'runtime',
    occurredAt: create(TimestampSchema, { seconds: 1700000000n }),
    evidence: '<script>alert("unsafe")</script>',
    matchedEvidence: 'bind: address already in use',
    inferred: true,
    evidenceAvailable: true,
    exitCode: 1n,
  })
}

const wrappers = new Set<VueWrapper>()
function mountPanel(
  permissions = [
    'game_server.view',
    'game_server.console',
    'game_server.settings',
    'game_server.metrics',
    'game_server.backup',
  ],
) {
  const wrapper = mount(GameServerDiagnosisPanel, {
    props: { serverId: 'server-a', status: Status.OFFLINE, permissions, readinessVisible: false },
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' },
      },
    },
  })
  wrappers.add(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  mocks.getGameServerDiagnosis.mockReset().mockResolvedValue({ diagnosis: report() })
})
afterEach(() => {
  wrappers.forEach((wrapper) => wrapper.unmount())
  wrappers.clear()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('latest server diagnosis', () => {
  it('renders escaped evidence and permitted links, retaining the failure while running', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('A required port is already in use')
    expect(wrapper.text()).toContain('Possible cause:')
    expect(wrapper.text()).toContain('<script>alert("unsafe")</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('details > summary').text()).toBe('Evidence and next steps')
    expect(wrapper.text()).toContain('Process exit code')
    expect(wrapper.findAll('a').map((link) => link.text())).toContain('Review server ports')
    const metricsLink = wrapper
      .findAll('a')
      .find((link) => link.text().includes('around this failure'))
    expect(metricsLink?.attributes('data-to')).toContain('2023-11-14T22:13:20.000Z')
    await wrapper.setProps({ status: Status.ONLINE })
    await flushPromises()
    expect(wrapper.text()).toContain('Last failure — server is now running')
    expect(mocks.getGameServerDiagnosis).toHaveBeenCalledTimes(2)
  })

  it('preserves the last response on refresh failure and recovers on the next poll', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    mocks.getGameServerDiagnosis.mockRejectedValueOnce(new Error('unavailable'))
    await vi.advanceTimersByTimeAsync(5_000)
    expect(wrapper.text()).toContain('A required port is already in use')
    expect(wrapper.text()).toContain('Report may be out of date')
    await vi.advanceTimersByTimeAsync(5_000)
    expect(wrapper.text()).not.toContain('Report may be out of date')
  })

  it('aborts hidden and unmounted requests, ignoring late evidence from a different server', async () => {
    let resolve!: (response: { diagnosis: ReturnType<typeof report> }) => void
    mocks.getGameServerDiagnosis.mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    const wrapper = mountPanel()
    const signal = mocks.getGameServerDiagnosis.mock.calls[0]?.[1].signal as AbortSignal
    await wrapper.setProps({ serverId: 'server-b' })
    expect(signal.aborted).toBe(true)
    await flushPromises()
    resolve({ diagnosis: create(GameServerDiagnosisSchema, { category: 'disk_full' }) })
    await flushPromises()
    expect(wrapper.text()).not.toContain('ran out of disk space')
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    const count = mocks.getGameServerDiagnosis.mock.calls.length
    await vi.advanceTimersByTimeAsync(10_000)
    expect(mocks.getGameServerDiagnosis).toHaveBeenCalledTimes(count)
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    const currentSignal = mocks.getGameServerDiagnosis.mock.calls.at(-1)?.[1].signal as AbortSignal
    wrapper.unmount()
    wrappers.delete(wrapper)
    expect(currentSignal.aborted).toBe(true)
  })

  it('hides restricted evidence and actions the operator cannot use', async () => {
    mocks.getGameServerDiagnosis.mockResolvedValue({
      diagnosis: create(GameServerDiagnosisSchema, { ...report(), evidenceRestricted: true }),
    })
    const wrapper = mountPanel(['game_server.view'])
    await flushPromises()
    expect(wrapper.text()).toContain('Console permission is required')
    expect(wrapper.find('pre').exists()).toBe(false)
    expect(wrapper.findAll('a')).toHaveLength(0)
  })

  it.each([
    { diagnosis: undefined, text: 'No failure recorded.' },
    {
      diagnosis: create(GameServerDiagnosisSchema, { stage: 'unknown_outcome' }),
      text: 'Start outcome unknown',
    },
    {
      diagnosis: create(GameServerDiagnosisSchema, { stage: 'launch', category: 'unknown' }),
      text: 'Final console evidence was not available',
    },
  ])(
    'distinguishes empty, unknown and unavailable evidence: $text',
    async ({ diagnosis, text }) => {
      mocks.getGameServerDiagnosis.mockResolvedValue({ diagnosis })
      const wrapper = mountPanel()
      await flushPromises()
      expect(wrapper.text()).toContain(text)
      expect(wrapper.text()).not.toContain('Process exit code')
    },
  )

  it('reuses the existing readiness workflow', async () => {
    mocks.getGameServerDiagnosis.mockResolvedValue({
      diagnosis: create(GameServerDiagnosisSchema, {
        category: 'incomplete_setup',
        stage: 'pre_start',
      }),
    })
    const wrapper = mountPanel()
    await wrapper.setProps({ readinessVisible: true })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('showReadiness')).toHaveLength(1)
  })

  it('distinguishes an unavailable first request from a confirmed empty report', async () => {
    mocks.getGameServerDiagnosis.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('Diagnosis is currently unavailable.')
    expect(wrapper.text()).not.toContain('No failure recorded.')
    mocks.getGameServerDiagnosis.mockResolvedValueOnce({})
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('No failure recorded.')
    mocks.getGameServerDiagnosis.mockRejectedValueOnce(new Error('unavailable'))
    await vi.advanceTimersByTimeAsync(5_000)
    expect(wrapper.text()).toContain('No failure was recorded at the last check.')
  })
})
