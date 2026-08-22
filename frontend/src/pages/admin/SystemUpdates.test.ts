import { create, type MessageInitShape } from '@bufbuild/protobuf'
import { timestampFromDate, type Timestamp } from '@bufbuild/protobuf/wkt'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GameServerSchema, NodeSchema } from '@/proto/shared_pb'
import {
  SystemUpdateAvailabilitySchema,
  SystemUpdateComponent,
  SystemUpdateJobSchema,
  SystemUpdateJobStatus,
  SystemUpdatePhase,
  SystemUpdateProgressSchema,
  type SystemUpdateAvailability,
  type SystemUpdateJob,
} from '@/proto/xylona_pb'
import SystemUpdates from './SystemUpdates.vue'

type Listener = (...args: unknown[]) => void

const mocks = vi.hoisted(() => {
  const listeners = new Map<string, Set<Listener>>()
  const eventBus = {
    on: vi.fn((event: string, listener: Listener) => {
      const eventListeners = listeners.get(event) ?? new Set<Listener>()
      eventListeners.add(listener)
      listeners.set(event, eventListeners)
    }),
    off: vi.fn((event: string, listener: Listener) => {
      listeners.get(event)?.delete(listener)
    }),
    emit(event: string, ...args: unknown[]) {
      for (const listener of listeners.get(event) ?? []) {
        listener(...args)
      }
    },
    reset() {
      listeners.clear()
      eventBus.on.mockClear()
      eventBus.off.mockClear()
    },
  }

  return {
    checkSystemUpdates: vi.fn(),
    getSystemUpdateJob: vi.fn(),
    listGameServers: vi.fn(),
    listNodes: vi.fn(),
    listSystemUpdateJobs: vi.fn(),
    startSystemUpdate: vi.fn(),
    getWebsocket: vi.fn(),
    reconnectWebsocket: vi.fn(),
    notifyCreate: vi.fn(),
    eventBus,
    connection: {} as {
      browserOnline: { value: boolean }
      connectionEpoch: { value: number }
      connectionStatus: { value: string }
    },
  }
})

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: ConnectError) => error.message,
  GetOrCreateXylonaWebsocketClient: mocks.getWebsocket,
  GetXylonaClient: () => ({
    checkSystemUpdates: mocks.checkSystemUpdates,
    getSystemUpdateJob: mocks.getSystemUpdateJob,
    listGameServers: mocks.listGameServers,
    listNodes: mocks.listNodes,
    listSystemUpdateJobs: mocks.listSystemUpdateJobs,
    startSystemUpdate: mocks.startSystemUpdate,
  }),
  reconnectControllerWebsocket: mocks.reconnectWebsocket,
  XylonaEventBus: mocks.eventBus,
}))

vi.mock('@/utils/websocket-connection', async () => {
  const { computed, readonly, ref } = await import('vue')
  const browserOnline = ref(true)
  const connectionEpoch = ref(1)
  const connectionStatus = ref('connected')
  mocks.connection.browserOnline = browserOnline
  mocks.connection.connectionEpoch = connectionEpoch
  mocks.connection.connectionStatus = connectionStatus
  return {
    websocketBrowserOnline: readonly(browserOnline),
    websocketConnectionEpoch: readonly(connectionEpoch),
    websocketConnectionStatus: readonly(connectionStatus),
    websocketStateAuthoritative: computed(() => connectionStatus.value === 'connected'),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    Notify: {
      create: mocks.notifyCreate,
    },
    useQuasar: () => ({
      screen: { lt: { md: false } },
    }),
  }
})

const QBtnStub = defineComponent({
  name: 'QBtn',
  inheritAttrs: false,
  props: {
    disable: { type: Boolean, default: false },
    label: { type: String, default: '' },
    loading: { type: Boolean, default: false },
  },
  emits: ['click'],
  template:
    '<button :disabled="disable || loading" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const QTableStub = defineComponent({
  name: 'QTable',
  props: {
    rows: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
  },
  template:
    '<div><span data-test="table-row-count">{{ rows.length }}</span><span data-test="table-loading">{{ loading }}</span></div>',
})

const passthrough = (name: string) =>
  defineComponent({
    name,
    template: '<div><slot /></div>',
  })

const mountOptions = {
  global: {
    directives: {
      closePopup: {},
    },
    stubs: {
      'q-badge': true,
      'q-btn': QBtnStub,
      'q-card': passthrough('QCard'),
      'q-card-actions': passthrough('QCardActions'),
      'q-card-section': passthrough('QCardSection'),
      'q-dialog': defineComponent({
        name: 'QDialog',
        props: {
          modelValue: { type: Boolean, default: false },
        },
        template: '<div v-if="modelValue"><slot /></div>',
      }),
      'q-icon': true,
      'q-item': passthrough('QItem'),
      'q-item-label': passthrough('QItemLabel'),
      'q-item-section': passthrough('QItemSection'),
      'q-linear-progress': true,
      'q-list': passthrough('QList'),
      'q-page': passthrough('QPage'),
      'q-separator': true,
      'q-spinner': true,
      'q-table': QTableStub,
      'q-td': passthrough('QTd'),
      'q-tooltip': passthrough('QTooltip'),
    },
  },
}

type SystemUpdatesViewModel = {
  activeJobs: SystemUpdateJob[]
  affectedServers: unknown[]
  availabilityError: string
  canSubmitSelected: boolean
  confirmOpen: boolean
  contextError: string
  gameServers: unknown[]
  jobs: SystemUpdateJob[]
  jobsError: string
  latestJobMessage: (job: SystemUpdateJob) => string
  loadAvailability: () => Promise<boolean>
  loadJobs: () => Promise<boolean>
  openConfirm: (update: SystemUpdateAvailability & { targetKey: string }) => Promise<void>
  reconcileJob: (jobId: string) => Promise<boolean>
  refreshAll: () => Promise<void>
  startSelectedUpdate: () => Promise<void>
  targetActionReason: (update: SystemUpdateAvailability & { targetKey: string }) => string
  terminalJobs: SystemUpdateJob[]
  updates: Array<SystemUpdateAvailability & { targetKey: string }>
}

const wrappers: VueWrapper[] = []

describe('SystemUpdates', () => {
  beforeEach(() => {
    vi.useRealTimers()
    mocks.eventBus.reset()
    mocks.checkSystemUpdates.mockReset()
    mocks.getSystemUpdateJob.mockReset()
    mocks.listGameServers.mockReset()
    mocks.listNodes.mockReset()
    mocks.listSystemUpdateJobs.mockReset()
    mocks.startSystemUpdate.mockReset()
    mocks.getWebsocket.mockReset()
    mocks.reconnectWebsocket.mockReset()
    mocks.notifyCreate.mockReset()

    mocks.connection.browserOnline.value = true
    mocks.connection.connectionEpoch.value = 1
    mocks.connection.connectionStatus.value = 'connected'

    mocks.checkSystemUpdates.mockResolvedValue({ updates: [controllerAvailability()] })
    mocks.listSystemUpdateJobs.mockResolvedValue({ jobs: [] })
    mocks.listNodes.mockResolvedValue({
      nodes: [create(NodeSchema, { id: 'local-node', name: 'Local', local: true })],
    })
    mocks.listGameServers.mockResolvedValue({
      gameServers: [
        create(GameServerSchema, {
          id: 'server-1',
          name: 'Primary Server',
          nodeId: 'local-node',
        }),
      ],
    })
    mocks.getSystemUpdateJob.mockResolvedValue({ job: undefined, events: [] })
    mocks.startSystemUpdate.mockResolvedValue({ job: undefined })
  })

  afterEach(() => {
    for (const wrapper of wrappers.splice(0)) {
      wrapper.unmount()
    }
    vi.useRealTimers()
  })

  it('stops loading the Resync button after the initial refresh completes', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const resyncButtons = wrapper
      .findAllComponents(QBtnStub)
      .filter((button) => button.props('label') === 'Resync')
    expect(resyncButtons).toHaveLength(1)
    expect(resyncButtons.every((button) => button.props('loading') === false)).toBe(true)
    expect(wrapper.get('[data-test="live-status-strip"]').text()).toContain(
      'No updates in progress',
    )
    expect(wrapper.find('#active-update-jobs-title').exists()).toBe(false)
  })

  it('keeps the idle layout stable while a Resync is pending', async () => {
    const wrapper = mountPage()
    await flushPromises()

    let resolveAvailability!: (value: { updates: SystemUpdateAvailability[] }) => void
    let resolveJobs!: (value: { jobs: SystemUpdateJob[] }) => void
    mocks.checkSystemUpdates.mockImplementationOnce(
      () =>
        new Promise<{ updates: SystemUpdateAvailability[] }>((resolve) => {
          resolveAvailability = resolve
        }),
    )
    mocks.listSystemUpdateJobs.mockImplementationOnce(
      () =>
        new Promise<{ jobs: SystemUpdateJob[] }>((resolve) => {
          resolveJobs = resolve
        }),
    )

    const resyncButton = wrapper
      .findAllComponents(QBtnStub)
      .find((button) => button.props('label') === 'Resync')
    if (!resyncButton) throw new Error('expected Resync button')
    await resyncButton.trigger('click')
    await nextTick()

    expect(wrapper.find('#active-update-jobs-title').exists()).toBe(false)
    expect(wrapper.get('[data-test="table-loading"]').text()).toBe('false')

    resolveAvailability({ updates: [controllerAvailability()] })
    resolveJobs({ jobs: [] })
    await flushPromises()
  })

  it('subscribes before hydration, coalesces unknown-job detail, and never regresses live progress', async () => {
    let resolveList!: (value: { jobs: SystemUpdateJob[] }) => void
    let resolveDetail!: (value: { job: SystemUpdateJob; events: never[] }) => void
    const listPromise = new Promise<{ jobs: SystemUpdateJob[] }>((resolve) => {
      resolveList = resolve
    })
    const detailPromise = new Promise<{ job: SystemUpdateJob; events: never[] }>((resolve) => {
      resolveDetail = resolve
    })
    mocks.listSystemUpdateJobs.mockReturnValueOnce(listPromise)
    mocks.getSystemUpdateJob.mockReturnValue(detailPromise)

    const wrapper = mountPage()
    await vi.waitFor(() => {
      expect(mocks.listSystemUpdateJobs).toHaveBeenCalledTimes(1)
    })

    expect(mocks.eventBus.on).toHaveBeenCalledWith('systemUpdateProgress', expect.any(Function))
    expect(mocks.eventBus.on.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.getWebsocket.mock.invocationCallOrder[0] ?? Number.MAX_SAFE_INTEGER,
    )

    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: 'job-live',
        component: SystemUpdateComponent.CONTROLLER,
        status: SystemUpdateJobStatus.DOWNLOADING,
        phase: SystemUpdatePhase.DOWNLOAD,
        progressPercent: 45,
        message: 'Downloading controller artifact',
        targetVersion: '2.0.0',
      }),
    )
    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: 'job-live',
        component: SystemUpdateComponent.CONTROLLER,
        status: SystemUpdateJobStatus.STAGING,
        phase: SystemUpdatePhase.STAGE,
        progressPercent: 68,
        message: 'Controller artifact staged',
        targetVersion: '2.0.0',
      }),
    )

    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(1)
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ timeoutMs: 15_000 }),
    )

    resolveDetail({
      job: updateJob({
        id: 'job-live',
        status: SystemUpdateJobStatus.DOWNLOADING,
        phase: SystemUpdatePhase.DOWNLOAD,
        progressPercent: 20,
      }),
      events: [],
    })
    resolveList({ jobs: [] })
    await flushPromises()

    const vm = viewModel(wrapper)
    expect(vm.activeJobs).toHaveLength(1)
    expect(vm.activeJobs[0]).toMatchObject({
      id: 'job-live',
      status: SystemUpdateJobStatus.STAGING,
      phase: SystemUpdatePhase.STAGE,
      progressPercent: 68,
    })
    expect(vm.latestJobMessage(firstItem(vm.activeJobs, 'active job'))).toBe(
      'Controller artifact staged',
    )
  })

  it('keeps progress received during a pending Start response and prevents duplicate submission', async () => {
    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)
    await vm.openConfirm(firstItem(vm.updates, 'update target'))

    let resolveStart!: (value: { job: SystemUpdateJob }) => void
    mocks.startSystemUpdate.mockReturnValueOnce(
      new Promise<{ job: SystemUpdateJob }>((resolve) => {
        resolveStart = resolve
      }),
    )
    const newerDetail = updateJob({
      id: 'pending-start-job',
      status: SystemUpdateJobStatus.DOWNLOADING,
      phase: SystemUpdatePhase.DOWNLOAD,
      progressPercent: 57,
      updatedAt: at(1_721_822_400),
    })
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: newerDetail, events: [] })

    const firstStart = vm.startSelectedUpdate()
    const duplicateStart = vm.startSelectedUpdate()
    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: 'pending-start-job',
        component: SystemUpdateComponent.CONTROLLER,
        nodeId: 'local-node',
        status: SystemUpdateJobStatus.RUNNING,
        phase: SystemUpdatePhase.PREFLIGHT,
        progressPercent: 12,
        message: 'Downloading the accepted update',
        targetVersion: '2.0.0',
      }),
    )
    await flushPromises()

    resolveStart({
      job: updateJob({
        id: 'pending-start-job',
        status: SystemUpdateJobStatus.PENDING,
        phase: SystemUpdatePhase.CHECK,
        progressPercent: 0,
        updatedAt: at(1_721_822_400),
      }),
    })
    await Promise.all([firstStart, duplicateStart])
    await flushPromises()

    expect(mocks.startSystemUpdate).toHaveBeenCalledTimes(1)
    expect(mocks.startSystemUpdate).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ timeoutMs: 20_000 }),
    )
    expect(vm.activeJobs).toHaveLength(1)
    expect(vm.activeJobs[0]).toMatchObject({
      id: 'pending-start-job',
      status: SystemUpdateJobStatus.DOWNLOADING,
      phase: SystemUpdatePhase.DOWNLOAD,
      progressPercent: 57,
    })
    expect(vm.latestJobMessage(firstItem(vm.activeJobs, 'active job'))).toBe(
      'Downloading the accepted update',
    )
  })

  it('does not let a slow older list overwrite a newer detail snapshot', async () => {
    const olderActive = updateJob({
      id: 'list-detail-order-job',
      status: SystemUpdateJobStatus.DOWNLOADING,
      phase: SystemUpdatePhase.DOWNLOAD,
      progressPercent: 40,
      updatedAt: at(1_721_822_400),
    })
    const newerDetail = updateJob({
      id: olderActive.id,
      status: SystemUpdateJobStatus.APPLYING,
      phase: SystemUpdatePhase.APPLY,
      progressPercent: 82,
      updatedAt: at(1_721_822_500),
    })
    mocks.listSystemUpdateJobs.mockResolvedValue({ jobs: [olderActive] })
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: olderActive, events: [] })

    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)

    let resolveList!: (value: { jobs: SystemUpdateJob[] }) => void
    mocks.listSystemUpdateJobs.mockReturnValueOnce(
      new Promise<{ jobs: SystemUpdateJob[] }>((resolve) => {
        resolveList = resolve
      }),
    )
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: newerDetail, events: [] })

    const slowList = vm.loadJobs()
    await vi.waitFor(() => {
      expect(mocks.listSystemUpdateJobs).toHaveBeenCalledTimes(2)
    })
    await vm.reconcileJob(olderActive.id)
    expect(firstItem(vm.activeJobs, 'active job')).toMatchObject({
      status: SystemUpdateJobStatus.APPLYING,
      phase: SystemUpdatePhase.APPLY,
      progressPercent: 82,
    })

    resolveList({ jobs: [olderActive] })
    await slowList
    await flushPromises()

    expect(firstItem(vm.activeJobs, 'active job')).toMatchObject({
      status: SystemUpdateJobStatus.APPLYING,
      phase: SystemUpdatePhase.APPLY,
      progressPercent: 82,
    })
  })

  it('supersedes a hung section refresh after a fresh connection and ignores late stale data', async () => {
    const staleAvailability = controllerAvailability({ latestVersion: '1.5.0' })
    const freshAvailability = controllerAvailability({ latestVersion: '3.0.0' })
    let staleSignal: AbortSignal | undefined
    let resolveStaleAvailability!: (value: { updates: SystemUpdateAvailability[] }) => void
    mocks.checkSystemUpdates
      .mockImplementationOnce((_request: unknown, options: { signal?: AbortSignal }) => {
        staleSignal = options.signal
        return new Promise<{ updates: SystemUpdateAvailability[] }>((resolve) => {
          resolveStaleAvailability = resolve
        })
      })
      .mockResolvedValue({ updates: [freshAvailability] })

    const wrapper = mountPage()
    await vi.waitFor(() => {
      expect(mocks.checkSystemUpdates).toHaveBeenCalledTimes(1)
    })

    mocks.connection.connectionEpoch.value += 1
    await vi.waitFor(() => {
      expect(mocks.checkSystemUpdates).toHaveBeenCalledTimes(2)
      expect(mocks.listSystemUpdateJobs).toHaveBeenCalledTimes(2)
      expect(mocks.listNodes).toHaveBeenCalledTimes(2)
      expect(mocks.listGameServers).toHaveBeenCalledTimes(2)
    })
    await flushPromises()

    expect(staleSignal?.aborted).toBe(true)
    expect(firstItem(viewModel(wrapper).updates, 'fresh update').latestVersion).toBe('3.0.0')

    resolveStaleAvailability({ updates: [staleAvailability] })
    await flushPromises()

    expect(firstItem(viewModel(wrapper).updates, 'fresh update').latestVersion).toBe('3.0.0')
  })

  it('keeps every last-good section when a preflight refresh partially fails', async () => {
    mocks.listSystemUpdateJobs.mockResolvedValue({
      jobs: [
        updateJob({
          id: 'completed-job',
          status: SystemUpdateJobStatus.SUCCEEDED,
          phase: SystemUpdatePhase.COMPLETE,
          progressPercent: 100,
        }),
      ],
    })
    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)
    const initialUpdate = firstItem(vm.updates, 'update target')

    mocks.checkSystemUpdates.mockRejectedValueOnce(
      new ConnectError('availability failed', Code.Unavailable),
    )
    mocks.listSystemUpdateJobs.mockRejectedValueOnce(
      new ConnectError('jobs failed', Code.Unavailable),
    )
    mocks.listGameServers.mockRejectedValueOnce(
      new ConnectError('server context failed', Code.Unavailable),
    )
    await vm.openConfirm(initialUpdate)
    await flushPromises()

    expect(vm.updates).toHaveLength(1)
    expect(vm.jobs).toHaveLength(1)
    expect(vm.gameServers).toHaveLength(1)
    expect(vm.availabilityError).toContain('availability failed')
    expect(vm.jobsError).toContain('jobs failed')
    expect(vm.contextError).toContain('server context failed')
    expect(vm.confirmOpen).toBe(false)
    expect(wrapper.text()).toContain('Affected game servers could not be verified.')
    expect(wrapper.text()).not.toContain('No game servers are assigned to this target.')
    expect(mocks.notifyCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        caption: expect.stringContaining('Fresh target, job, and affected-server context'),
      }),
    )
  })

  it('polls active jobs single-flight and recovers controller completion after restart', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'))
    const active = updateJob({
      id: 'controller-restart',
      status: SystemUpdateJobStatus.RESTARTING,
      phase: SystemUpdatePhase.RESTART,
      progressPercent: 92,
    })
    const succeeded = updateJob({
      id: 'controller-restart',
      status: SystemUpdateJobStatus.SUCCEEDED,
      phase: SystemUpdatePhase.COMPLETE,
      progressPercent: 100,
    })
    mocks.listSystemUpdateJobs.mockResolvedValue({ jobs: [active] })
    mocks.getSystemUpdateJob
      .mockResolvedValueOnce({ job: active, events: [] })
      .mockResolvedValue({ job: succeeded, events: [] })

    const wrapper = mountPage()
    await flushPromises()

    expect(viewModel(wrapper).activeJobs).toHaveLength(1)
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    const vm = viewModel(wrapper)
    expect(vm.activeJobs).toHaveLength(0)
    expect(vm.terminalJobs).toHaveLength(1)
    expect(wrapper.find('[data-test="controller-reload-prompt"]').exists()).toBe(true)
    expect(mocks.checkSystemUpdates.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('keeps a failed unknown terminal detail lookup scheduled until persisted recovery succeeds', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'))
    const wrapper = mountPage()
    await flushPromises()

    const succeeded = updateJob({
      id: 'unknown-terminal-job',
      status: SystemUpdateJobStatus.SUCCEEDED,
      phase: SystemUpdatePhase.COMPLETE,
      progressPercent: 100,
    })
    mocks.getSystemUpdateJob
      .mockRejectedValueOnce(new ConnectError('controller restarting', Code.Unavailable))
      .mockResolvedValue({ job: succeeded, events: [] })

    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: succeeded.id,
        component: SystemUpdateComponent.CONTROLLER,
        status: SystemUpdateJobStatus.SUCCEEDED,
        phase: SystemUpdatePhase.COMPLETE,
        progressPercent: 100,
        message: 'Controller update complete',
        targetVersion: succeeded.targetVersion,
      }),
    )
    await flushPromises()
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(2)
    expect(viewModel(wrapper).terminalJobs[0]?.status).toBe(SystemUpdateJobStatus.SUCCEEDED)
    expect(wrapper.find('[data-test="controller-reload-prompt"]').exists()).toBe(true)
  })

  it('supersedes a hung poll on a fresh connection and ignores its late completion', async () => {
    vi.useFakeTimers()
    const active = updateJob({
      id: 'deferred-poll-job',
      status: SystemUpdateJobStatus.RESTARTING,
      phase: SystemUpdatePhase.RESTART,
      progressPercent: 90,
      updatedAt: at(1_721_822_400),
    })
    const staleCompletion = updateJob({
      id: active.id,
      status: SystemUpdateJobStatus.RUNNING,
      phase: SystemUpdatePhase.PREFLIGHT,
      progressPercent: 15,
      updatedAt: at(1_721_822_300),
    })
    const freshTerminal = updateJob({
      id: active.id,
      status: SystemUpdateJobStatus.SUCCEEDED,
      phase: SystemUpdatePhase.COMPLETE,
      progressPercent: 100,
      updatedAt: at(1_721_822_500),
    })
    mocks.listSystemUpdateJobs.mockResolvedValue({ jobs: [active] })
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: active, events: [] })

    const wrapper = mountPage()
    await flushPromises()
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(1)

    let resolvePoll!: (value: { job: SystemUpdateJob; events: never[] }) => void
    let pollSignal: AbortSignal | undefined
    mocks.getSystemUpdateJob.mockImplementationOnce(
      (_request: unknown, options: { signal?: AbortSignal }) => {
        pollSignal = options.signal
        return new Promise<{ job: SystemUpdateJob; events: never[] }>((resolve) => {
          resolvePoll = resolve
        })
      },
    )
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: freshTerminal, events: [] })

    vi.advanceTimersByTime(10_000)
    await Promise.resolve()
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(2)

    mocks.connection.connectionEpoch.value += 1
    await flushPromises()
    expect(pollSignal?.aborted).toBe(true)
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(3)
    expect(firstItem(viewModel(wrapper).terminalJobs, 'terminal job').status).toBe(
      SystemUpdateJobStatus.SUCCEEDED,
    )

    resolvePoll({ job: staleCompletion, events: [] })
    await flushPromises()

    expect(firstItem(viewModel(wrapper).terminalJobs, 'terminal job').status).toBe(
      SystemUpdateJobStatus.SUCCEEDED,
    )
    expect(mocks.getSystemUpdateJob).toHaveBeenCalledTimes(3)
  })

  it('aborts an outstanding unknown-job detail request on unmount', async () => {
    const wrapper = mountPage()
    await flushPromises()

    let detailSignal: AbortSignal | undefined
    let resolveDetail!: (value: { job: SystemUpdateJob; events: never[] }) => void
    mocks.getSystemUpdateJob.mockImplementationOnce(
      (_request: unknown, options: { signal?: AbortSignal }) => {
        detailSignal = options.signal
        return new Promise<{ job: SystemUpdateJob; events: never[] }>((resolve) => {
          resolveDetail = resolve
        })
      },
    )
    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: 'unmount-detail-job',
        component: SystemUpdateComponent.NODE,
        nodeId: 'node-2',
        status: SystemUpdateJobStatus.RUNNING,
        phase: SystemUpdatePhase.PREFLIGHT,
        progressPercent: 5,
      }),
    )
    await Promise.resolve()

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    expect(detailSignal?.aborted).toBe(true)

    resolveDetail({
      job: updateJob({
        id: 'unmount-detail-job',
        component: SystemUpdateComponent.NODE,
        nodeId: 'node-2',
        status: SystemUpdateJobStatus.RUNNING,
        phase: SystemUpdatePhase.PREFLIGHT,
      }),
      events: [],
    })
    await flushPromises()
  })

  it('lets persisted terminal state beat a newer live overlay and retains the target lock until then', async () => {
    vi.useFakeTimers()
    const active = updateJob({
      id: 'terminal-authority-job',
      status: SystemUpdateJobStatus.RESTARTING,
      phase: SystemUpdatePhase.RESTART,
      progressPercent: 94,
    })
    const failed = updateJob({
      id: active.id,
      status: SystemUpdateJobStatus.FAILED,
      phase: SystemUpdatePhase.FAILURE,
      progressPercent: 94,
      error: 'Controller handoff did not complete',
    })
    mocks.listSystemUpdateJobs.mockResolvedValue({ jobs: [active] })
    mocks.getSystemUpdateJob.mockResolvedValueOnce({ job: active, events: [] })

    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)
    const update = firstItem(vm.updates, 'update target')

    let resolvePoll!: (value: { job: SystemUpdateJob; events: never[] }) => void
    mocks.getSystemUpdateJob.mockImplementationOnce(
      () =>
        new Promise<{ job: SystemUpdateJob; events: never[] }>((resolve) => {
          resolvePoll = resolve
        }),
    )
    vi.advanceTimersByTime(10_000)
    await Promise.resolve()

    mocks.eventBus.emit(
      'systemUpdateProgress',
      create(SystemUpdateProgressSchema, {
        jobId: active.id,
        component: SystemUpdateComponent.CONTROLLER,
        nodeId: 'local-node',
        status: SystemUpdateJobStatus.SUCCEEDED,
        phase: SystemUpdatePhase.COMPLETE,
        progressPercent: 100,
        message: 'Live stream reported success',
        targetVersion: active.targetVersion,
      }),
    )
    await nextTick()

    mocks.listSystemUpdateJobs.mockResolvedValueOnce({ jobs: [] })
    await vm.loadJobs()
    expect(vm.targetActionReason(update)).toContain('already active')
    expect(vm.jobs).toHaveLength(1)

    resolvePoll({ job: failed, events: [] })
    await flushPromises()

    expect(vm.terminalJobs).toHaveLength(1)
    expect(vm.terminalJobs[0]?.status).toBe(SystemUpdateJobStatus.FAILED)
    expect(vm.terminalJobs[0]?.error).toBe('Controller handoff did not complete')
    expect(wrapper.find('[data-test="controller-reload-prompt"]').exists()).toBe(false)
  })

  it('disables an open confirmation when the browser goes offline or availability refresh fails', async () => {
    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)
    await vm.openConfirm(firstItem(vm.updates, 'update target'))
    expect(vm.confirmOpen).toBe(true)
    expect(vm.canSubmitSelected).toBe(true)

    mocks.connection.browserOnline.value = false
    await nextTick()
    expect(vm.canSubmitSelected).toBe(false)
    await vm.startSelectedUpdate()
    expect(mocks.startSystemUpdate).not.toHaveBeenCalled()

    mocks.connection.browserOnline.value = true
    await flushPromises()
    expect(vm.canSubmitSelected).toBe(true)

    mocks.checkSystemUpdates.mockRejectedValueOnce(
      new ConnectError('availability refresh failed', Code.Unavailable),
    )
    await vm.loadAvailability()
    expect(vm.availabilityError).toContain('availability refresh failed')
    expect(vm.canSubmitSelected).toBe(false)
    await vm.startSelectedUpdate()
    expect(mocks.startSystemUpdate).not.toHaveBeenCalled()
  })

  it('does not duplicate an ambiguous Start request and recovers the accepted job', async () => {
    const wrapper = mountPage()
    await flushPromises()
    const vm = viewModel(wrapper)
    const update = firstItem(vm.updates, 'update target')

    mocks.listSystemUpdateJobs.mockResolvedValueOnce({ jobs: [] }).mockResolvedValueOnce({
      jobs: [
        updateJob({
          id: 'accepted-job',
          status: SystemUpdateJobStatus.PENDING,
          phase: SystemUpdatePhase.CHECK,
          progressPercent: 0,
        }),
      ],
    })
    mocks.startSystemUpdate.mockRejectedValueOnce(
      new ConnectError('response connection closed', Code.Unavailable),
    )

    await vm.openConfirm(update)
    expect(vm.confirmOpen).toBe(true)

    const firstStart = vm.startSelectedUpdate()
    const duplicateStart = vm.startSelectedUpdate()
    await Promise.all([firstStart, duplicateStart])
    await flushPromises()

    expect(mocks.startSystemUpdate).toHaveBeenCalledTimes(1)
    expect(mocks.startSystemUpdate).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ timeoutMs: 20_000 }),
    )
    expect(vm.activeJobs).toHaveLength(1)
    expect(vm.activeJobs[0]?.id).toBe('accepted-job')
    expect(vm.targetActionReason(update)).toContain('already active')
    expect(vm.confirmOpen).toBe(false)
    expect(mocks.notifyCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        caption: expect.stringContaining('persisted job state'),
      }),
    )
  })
})

function mountPage(): VueWrapper {
  const wrapper = mount(SystemUpdates, mountOptions)
  wrappers.push(wrapper)
  return wrapper
}

function viewModel(wrapper: VueWrapper): SystemUpdatesViewModel {
  return wrapper.vm as unknown as SystemUpdatesViewModel
}

function firstItem<T>(items: T[], description: string): T {
  const item = items[0]
  if (item === undefined) {
    throw new Error(`expected ${description}`)
  }
  return item
}

function controllerAvailability(
  overrides: MessageInitShape<typeof SystemUpdateAvailabilitySchema> = {},
): SystemUpdateAvailability {
  return create(SystemUpdateAvailabilitySchema, {
    component: SystemUpdateComponent.CONTROLLER,
    currentVersion: '1.0.0',
    latestVersion: '2.0.0',
    updateAvailable: true,
    updateable: true,
    os: 'windows',
    architecture: 'amd64',
    artifactName: 'xylona_windows_amd64.zip',
    artifactSha256: 'abc123',
    ...overrides,
  })
}

function at(unixSeconds: number): Timestamp {
  return timestampFromDate(new Date(unixSeconds * 1_000))
}

type UpdateJobInit = MessageInitShape<typeof SystemUpdateJobSchema>

function updateJob(
  overrides: UpdateJobInit & Required<Pick<UpdateJobInit, 'id' | 'status' | 'phase'>>,
): SystemUpdateJob {
  return create(SystemUpdateJobSchema, {
    component: SystemUpdateComponent.CONTROLLER,
    nodeId: 'local-node',
    currentVersion: '1.0.0',
    targetVersion: '2.0.0',
    progressPercent: 10,
    requestedByUserId: 'user-1',
    requestedByUserName: 'admin',
    createdAt: at(1_721_822_400),
    updatedAt: at(1_721_822_401),
    ...overrides,
  })
}
