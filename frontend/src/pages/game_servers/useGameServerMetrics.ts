import { create, toJsonString } from '@bufbuild/protobuf'
import { TimestampSchema, type Timestamp } from '@bufbuild/protobuf/wkt'
import { ConnectError } from '@connectrpc/connect'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import {
  GameServerSchema,
  type GameServer,
  type GameServerLifecycleHistoryEvent,
  type GameServerMetricsHistoryPoint,
  type GameServerOperationHistoryEvent,
} from '@/proto/shared_pb'
import {
  GetGameServerMetricsHistoryRequestSchema,
  GetGameServerRequestSchema,
} from '@/proto/xylona_pb'
import {
  Request_Type,
  RequestSchema,
  type AllNodeMetrics,
  type AllServersMetrics,
  type GameServerMetrics,
} from '@/proto/websocket_pb'
import {
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'
import {
  calculateMetricCapacity,
  deriveMetricsViewState,
  getMetricsRangeRequest,
  getMetricsSubscriptionTransition,
  LatestRequestGuard,
  mergeMetricSamples,
  mergeMetricHistoryWithLiveTail,
  type MetricCapacity,
  type MetricSample,
  type MetricsRangeKey,
} from './game-server-metrics'

export interface MetricsTimelineEvent {
  id: string
  timestampMs: number
  kind: 'lifecycle' | 'operation'
  title: string
  detail: string
  tone: 'neutral' | 'positive' | 'warning' | 'negative'
}

interface UseGameServerMetricsOptions {
  gameServerId: Ref<string>
  initialRange?: MetricsRangeKey
}

function timestampToMs(timestamp: Timestamp | undefined): number | null {
  if (!timestamp) return null
  return Number(timestamp.seconds) * 1000 + Math.floor(timestamp.nanos / 1_000_000)
}

function optionalNumber(value: number | bigint | undefined): number | null {
  return value === undefined ? null : Number(value)
}

function valueWhen(valid: boolean, value: number | bigint): number | null {
  return valid ? Number(value) : null
}

function validSampleCount(value: number, sampleCount: number): number {
  return Math.min(Math.max(value, 0), sampleCount)
}

export function normalizeMetricPoint(input: GameServerMetricsHistoryPoint): MetricSample {
  const timestampMs = timestampToMs(input.timestamp) ?? 0
  const rss = input.memoryRssBytes > 0n ? input.memoryRssBytes : input.memoryBytes
  const sampleCount = Math.max(input.sampleCount, 1)
  const processValidSampleCount = validSampleCount(input.availableSampleCount, sampleCount)
  const cpuValidSampleCount = validSampleCount(input.cpuValidSampleCount, sampleCount)
  const volumeValidSampleCount = validSampleCount(input.volumeValidSampleCount, sampleCount)
  const ioValidSampleCount = validSampleCount(input.ioValidSampleCount, sampleCount)
  const connectionValidSampleCount = validSampleCount(input.connectionValidSampleCount, sampleCount)
  const querySuccessfulSampleCount = validSampleCount(input.querySuccessfulSampleCount, sampleCount)
  const queryDurationValidSampleCount = validSampleCount(
    input.queryDurationValidSampleCount,
    sampleCount,
  )
  const serverFpsValidSampleCount = validSampleCount(input.serverFpsValidSampleCount, sampleCount)
  const serverFrameTimeValidSampleCount = validSampleCount(
    input.serverFrameTimeValidSampleCount,
    sampleCount,
  )
  const processAvailable = processValidSampleCount > 0
  const cpuAvailable = input.cpuValid && cpuValidSampleCount > 0
  const diskAvailable = input.volumeValid && volumeValidSampleCount > 0
  const ioAvailable = input.ioValid && ioValidSampleCount > 0
  const connectionAvailable = input.connectionCountValid && connectionValidSampleCount > 0
  const queryDurationAvailable = queryDurationValidSampleCount > 0
  const serverFpsAvailable = serverFpsValidSampleCount > 0
  const serverFrameTimeAvailable = serverFrameTimeValidSampleCount > 0

  return {
    timestampMs,
    cpuAverage: valueWhen(cpuAvailable, input.cpuPercent),
    cpuMinimum: cpuAvailable ? (input.cpuPercentMin ?? input.cpuPercent) : null,
    cpuMaximum: cpuAvailable ? (input.cpuPercentMax ?? input.cpuPercent) : null,
    nodeCpuCores: optionalNumber(input.nodeCpuCores),
    memoryRssAverage: processAvailable ? Number(rss) : null,
    memoryRssMinimum: processAvailable ? optionalNumber(input.memoryRssBytesMin ?? rss) : null,
    memoryRssMaximum: processAvailable ? optionalNumber(input.memoryRssBytesMax ?? rss) : null,
    memoryPercentAverage: processAvailable ? input.memoryPercent : null,
    memoryPercentMinimum: processAvailable ? (input.memoryPercentMin ?? input.memoryPercent) : null,
    memoryPercentMaximum: processAvailable ? (input.memoryPercentMax ?? input.memoryPercent) : null,
    nodeMemoryUsedBytes: optionalNumber(input.nodeMemoryUsedBytes),
    nodeMemoryTotalBytes: optionalNumber(input.nodeMemoryTotalBytes),
    configuredMemoryBytes: optionalNumber(input.configuredMemoryBytes),
    diskUsageAverage: diskAvailable ? Number(input.diskUsageBytes) : null,
    diskUsageMinimum: diskAvailable
      ? optionalNumber(input.diskUsageBytesMin ?? input.diskUsageBytes)
      : null,
    diskUsageMaximum: diskAvailable
      ? optionalNumber(input.diskUsageBytesMax ?? input.diskUsageBytes)
      : null,
    volumeTotalBytes: optionalNumber(input.volumeTotalBytes),
    volumeFreeBytes: optionalNumber(input.volumeFreeBytes),
    volumePercent: optionalNumber(input.volumePercent),
    volumeValid: diskAvailable,
    ioValid: ioAvailable,
    connectionCountValid: connectionAvailable,
    diskMeasuredAtMs: timestampToMs(input.diskMeasuredAt),
    ioReadAverage: ioAvailable ? input.ioReadRate : null,
    ioReadMinimum: ioAvailable ? (input.ioReadRateMin ?? input.ioReadRate) : null,
    ioReadMaximum: ioAvailable ? (input.ioReadRateMax ?? input.ioReadRate) : null,
    ioWriteAverage: ioAvailable ? input.ioWriteRate : null,
    ioWriteMinimum: ioAvailable ? (input.ioWriteRateMin ?? input.ioWriteRate) : null,
    ioWriteMaximum: ioAvailable ? (input.ioWriteRateMax ?? input.ioWriteRate) : null,
    connectionAverage: connectionAvailable ? input.connectionCount : null,
    connectionMinimum: connectionAvailable
      ? (input.connectionCountMin ?? input.connectionCount)
      : null,
    connectionMaximum: connectionAvailable
      ? (input.connectionCountMax ?? input.connectionCount)
      : null,
    playerAverage: input.playerCountValid ? input.playerCount : null,
    playerMinimum: input.playerCountValid ? (input.playerCountMin ?? input.playerCount) : null,
    playerMaximum: input.playerCountValid ? (input.playerCountMax ?? input.playerCount) : null,
    playerCapacity: optionalNumber(input.playerCapacity),
    querySupported: input.querySupported ?? null,
    querySuccess: input.querySuccess ?? null,
    queryDurationAverage: queryDurationAvailable ? optionalNumber(input.queryDurationMs) : null,
    queryDurationMinimum: queryDurationAvailable
      ? optionalNumber(input.queryDurationMsMin ?? input.queryDurationMs)
      : null,
    queryDurationMaximum: queryDurationAvailable
      ? optionalNumber(input.queryDurationMsMax ?? input.queryDurationMs)
      : null,
    queryCheckedAtMs: timestampToMs(input.queryCheckedAt),
    serverFpsAverage: serverFpsAvailable ? optionalNumber(input.serverFps) : null,
    serverFpsMinimum: serverFpsAvailable
      ? optionalNumber(input.serverFpsMin ?? input.serverFps)
      : null,
    serverFpsMaximum: serverFpsAvailable
      ? optionalNumber(input.serverFpsMax ?? input.serverFps)
      : null,
    frameTimeAverage: serverFrameTimeAvailable ? optionalNumber(input.serverFrameTimeMs) : null,
    frameTimeMinimum: serverFrameTimeAvailable
      ? optionalNumber(input.serverFrameTimeMsMin ?? input.serverFrameTimeMs)
      : null,
    frameTimeMaximum: serverFrameTimeAvailable
      ? optionalNumber(input.serverFrameTimeMsMax ?? input.serverFrameTimeMs)
      : null,
    uptimeSeconds: optionalNumber(input.serverUptimeSeconds),
    processStatus: input.processStatus,
    collectionStatus: input.collectionStatus,
    processCollectedAtMs: timestampToMs(input.processCollectedAt),
    granularitySeconds: input.granularitySeconds,
    sampleCount,
    availableSampleCount: processValidSampleCount,
    cpuValidSampleCount,
    volumeValidSampleCount,
    ioValidSampleCount,
    connectionValidSampleCount,
    querySuccessfulSampleCount,
    queryDurationValidSampleCount,
    serverFpsValidSampleCount,
    serverFrameTimeValidSampleCount,
    availabilityRatio: input.availabilityRatio,
    nodeId: input.nodeId,
    source: 'history',
  }
}

function lifecycleTone(event: GameServerLifecycleHistoryEvent): MetricsTimelineEvent['tone'] {
  if (event.exitCode !== undefined && event.exitCode !== 0 && !event.intentionalStop)
    return 'negative'
  if (event.status.toLowerCase() === 'online') return 'positive'
  if (event.intentionalStop) return 'neutral'
  return 'warning'
}

function normalizeTimeline(
  lifecycleEvents: readonly GameServerLifecycleHistoryEvent[],
  operationEvents: readonly GameServerOperationHistoryEvent[],
): MetricsTimelineEvent[] {
  const lifecycle = lifecycleEvents.map((raw) => {
    const event = raw
    const detailParts = [
      event.previousStatus !== '' ? `${event.previousStatus} to ${event.status}` : event.status,
      event.intentionalStop ? 'intentional' : '',
      event.exitCode !== undefined ? `exit ${event.exitCode}` : '',
    ].filter(Boolean)
    return {
      id: event.id,
      timestampMs: timestampToMs(event.observedAt) ?? 0,
      kind: 'lifecycle' as const,
      title: `Server ${event.status || 'status changed'}`,
      detail: detailParts.join(' · '),
      tone: lifecycleTone(event),
    }
  })
  const operations = operationEvents.map((raw) => {
    const event = raw
    const completedAt = timestampToMs(event.completedAt)
    const startedAt = timestampToMs(event.startedAt)
    const outcome = event.outcome.toLowerCase()
    return {
      id: event.id,
      timestampMs: completedAt ?? startedAt ?? 0,
      kind: 'operation' as const,
      title: event.operation || 'Server operation',
      detail: [event.phase, event.outcome, event.source].filter(Boolean).join(' · '),
      tone:
        outcome === 'failed' || outcome === 'error'
          ? ('negative' as const)
          : outcome === 'complete' ||
              outcome === 'completed' ||
              outcome === 'success' ||
              outcome === 'succeeded'
            ? ('positive' as const)
            : ('neutral' as const),
    }
  })

  return [...lifecycle, ...operations]
    .filter((event) => event.timestampMs > 0)
    .sort((left, right) => right.timestampMs - left.timestampMs)
}

function resolutionLabel(resolution: number | undefined, mixed: boolean): string {
  if (mixed || resolution === 4) return 'Mixed raw and hourly samples'
  if (resolution === 1) return 'Raw samples'
  if (resolution === 2) return 'Hourly rollups'
  if (resolution === 3) return 'Downsampled for display'
  return 'Automatic resolution'
}

export function normalizeLiveMetricPoint(
  metrics: GameServerMetrics,
  input: {
    timestampMs: number
    collectedAtMs: number
    latest: MetricSample | null
    nodeMemoryUsedBytes: number | null
    nodeMemoryTotalBytes: number | null
    configuredMemoryBytes: number | null
    nodeId: string
  },
): MetricSample {
  const rss = Number(metrics.memoryWorkingSetBytes || metrics.memoryBytes)
  const processAvailable = metrics.metricsValid
  const ioAvailable = metrics.ioValid
  const connectionAvailable = metrics.connectionCountValid
  const diskMeasuredAtMs = timestampToMs(metrics.diskMeasuredAt)

  return {
    timestampMs: input.timestampMs,
    cpuAverage: metrics.cpuValid ? metrics.cpuPercent : null,
    cpuMinimum: metrics.cpuValid ? metrics.cpuPercent : null,
    cpuMaximum: metrics.cpuValid ? metrics.cpuPercent : null,
    nodeCpuCores: metrics.cpuCores > 0 ? metrics.cpuCores : (input.latest?.nodeCpuCores ?? null),
    memoryRssAverage: processAvailable ? rss : null,
    memoryRssMinimum: processAvailable ? rss : null,
    memoryRssMaximum: processAvailable ? rss : null,
    memoryPercentAverage: processAvailable ? metrics.memoryPercent : null,
    memoryPercentMinimum: processAvailable ? metrics.memoryPercent : null,
    memoryPercentMaximum: processAvailable ? metrics.memoryPercent : null,
    nodeMemoryUsedBytes: input.nodeMemoryUsedBytes ?? input.latest?.nodeMemoryUsedBytes ?? null,
    nodeMemoryTotalBytes: input.nodeMemoryTotalBytes ?? input.latest?.nodeMemoryTotalBytes ?? null,
    configuredMemoryBytes:
      input.configuredMemoryBytes ?? input.latest?.configuredMemoryBytes ?? null,
    diskUsageAverage: metrics.diskValid ? Number(metrics.diskUsageBytes) : null,
    diskUsageMinimum: metrics.diskValid ? Number(metrics.diskUsageBytes) : null,
    diskUsageMaximum: metrics.diskValid ? Number(metrics.diskUsageBytes) : null,
    volumeTotalBytes:
      diskMeasuredAtMs !== null
        ? Number(metrics.diskTotalBytes)
        : (input.latest?.volumeTotalBytes ?? null),
    volumeFreeBytes:
      diskMeasuredAtMs !== null
        ? Number(metrics.diskFreeBytes)
        : (input.latest?.volumeFreeBytes ?? null),
    volumePercent:
      diskMeasuredAtMs !== null ? metrics.diskPercent : (input.latest?.volumePercent ?? null),
    volumeValid: metrics.diskValid,
    ioValid: ioAvailable,
    connectionCountValid: connectionAvailable,
    diskMeasuredAtMs: diskMeasuredAtMs ?? input.latest?.diskMeasuredAtMs ?? null,
    ioReadAverage: ioAvailable ? metrics.ioReadRate : null,
    ioReadMinimum: ioAvailable ? metrics.ioReadRate : null,
    ioReadMaximum: ioAvailable ? metrics.ioReadRate : null,
    ioWriteAverage: ioAvailable ? metrics.ioWriteRate : null,
    ioWriteMinimum: ioAvailable ? metrics.ioWriteRate : null,
    ioWriteMaximum: ioAvailable ? metrics.ioWriteRate : null,
    connectionAverage: connectionAvailable ? metrics.connectionCount : null,
    connectionMinimum: connectionAvailable ? metrics.connectionCount : null,
    connectionMaximum: connectionAvailable ? metrics.connectionCount : null,
    playerAverage: null,
    playerMinimum: null,
    playerMaximum: null,
    playerCapacity: null,
    querySupported: null,
    querySuccess: null,
    queryDurationAverage: null,
    queryDurationMinimum: null,
    queryDurationMaximum: null,
    queryCheckedAtMs: null,
    serverFpsAverage: null,
    serverFpsMinimum: null,
    serverFpsMaximum: null,
    frameTimeAverage: null,
    frameTimeMinimum: null,
    frameTimeMaximum: null,
    uptimeSeconds: Number(metrics.uptimeSeconds),
    processStatus: metrics.processStatus,
    collectionStatus: metrics.collectionStatus,
    processCollectedAtMs: input.collectedAtMs,
    granularitySeconds: 3,
    sampleCount: 1,
    availableSampleCount: processAvailable ? 1 : 0,
    cpuValidSampleCount: metrics.cpuValid ? 1 : 0,
    volumeValidSampleCount: metrics.diskValid ? 1 : 0,
    ioValidSampleCount: ioAvailable ? 1 : 0,
    connectionValidSampleCount: connectionAvailable ? 1 : 0,
    querySuccessfulSampleCount: 0,
    queryDurationValidSampleCount: 0,
    serverFpsValidSampleCount: 0,
    serverFrameTimeValidSampleCount: 0,
    availabilityRatio: processAvailable ? 1 : 0,
    nodeId: input.nodeId,
    source: 'live',
  }
}

export function useGameServerMetrics({ gameServerId, initialRange }: UseGameServerMetricsOptions) {
  const selectedRange = ref<MetricsRangeKey>(initialRange ?? '1h')
  const samples = ref<MetricSample[]>([])
  const timeline = ref<MetricsTimelineEvent[]>([])
  const gameServer = ref<GameServer>(create(GameServerSchema))
  const loading = ref(false)
  const error = ref('')
  const resolution = ref('Automatic resolution')
  const sampleIntervalSeconds = ref(0)
  const mixedResolution = ref(false)
  const nodeMemoryUsedBytes = ref<number | null>(null)
  const nodeMemoryTotalBytes = ref<number | null>(null)
  const liveProcessMetrics = ref<GameServerMetrics | null>(null)
  const guard = new LatestRequestGuard()
  let historyRefreshTimer: ReturnType<typeof setInterval> | undefined
  let subscribedGameServerId = ''
  let subscriptionMounted = false

  const rangeRequest = computed(() => getMetricsRangeRequest(selectedRange.value))
  const latestSample = computed(() => samples.value[samples.value.length - 1] ?? null)
  const viewState = computed(() =>
    deriveMetricsViewState({
      loading: loading.value,
      error: error.value,
      samples: samples.value,
      websocketConnected: websocketStateAuthoritative.value,
    }),
  )
  const currentCapacity = computed<MetricCapacity>(() => {
    const latest = latestSample.value
    return calculateMetricCapacity({
      processRssBytes: latest?.memoryRssAverage ?? null,
      configuredTargetBytes:
        Number(gameServer.value.maxMemoryMb) > 0
          ? Number(gameServer.value.maxMemoryMb) * 1024 * 1024
          : (latest?.configuredMemoryBytes ?? null),
      nodeUsedBytes: nodeMemoryUsedBytes.value ?? latest?.nodeMemoryUsedBytes ?? null,
      nodeTotalBytes: nodeMemoryTotalBytes.value ?? latest?.nodeMemoryTotalBytes ?? null,
    })
  })

  function resetServerScopedState(): void {
    guard.invalidate()
    samples.value = []
    timeline.value = []
    gameServer.value = create(GameServerSchema)
    loading.value = false
    error.value = ''
    resolution.value = 'Automatic resolution'
    sampleIntervalSeconds.value = 0
    mixedResolution.value = false
    nodeMemoryUsedBytes.value = null
    nodeMemoryTotalBytes.value = null
    liveProcessMetrics.value = null
  }

  async function fetchMetrics(): Promise<void> {
    const requestSequence = guard.begin()
    const serverId = gameServerId.value
    const range = getMetricsRangeRequest(selectedRange.value)
    loading.value = true
    error.value = ''

    const since = create(TimestampSchema, { seconds: BigInt(Math.floor(range.sinceMs / 1000)) })
    const until = create(TimestampSchema, { seconds: BigInt(Math.floor(range.untilMs / 1000)) })
    const historyRequest = create(GetGameServerMetricsHistoryRequestSchema, {
      gameServerId: serverId,
      since,
      until,
      maxPoints: range.maxPoints,
    })

    try {
      const [historyResponse, gameServerResponse] = await Promise.all([
        GetXylonaClient().getGameServerMetricsHistory(historyRequest),
        GetXylonaClient().getGameServer(
          create(GetGameServerRequestSchema, {
            id: serverId,
          }),
        ),
      ])
      if (!guard.isCurrent(requestSequence) || serverId !== gameServerId.value) return

      let nextSamples = historyResponse.points
        .map(normalizeMetricPoint)
        .filter((sample) => sample.timestampMs > 0)
      if (range.live) {
        nextSamples = mergeMetricHistoryWithLiveTail(
          nextSamples,
          samples.value,
          range.sinceMs,
          range.maxPoints,
        )
      }
      samples.value = nextSamples
      timeline.value = normalizeTimeline(
        historyResponse.lifecycleEvents,
        historyResponse.operationEvents,
      )
      mixedResolution.value = historyResponse.hasMixedResolution
      resolution.value = resolutionLabel(historyResponse.resolution, mixedResolution.value)
      sampleIntervalSeconds.value = historyResponse.sampleIntervalSeconds
      if (gameServerResponse.gameServer) gameServer.value = gameServerResponse.gameServer
    } catch (unknownError: unknown) {
      if (!guard.isCurrent(requestSequence)) return
      const connectError = ConnectError.from(unknownError)
      error.value = ConnectErrorToString(connectError)
      console.error('Failed to fetch game server metrics:', connectError.message)
    } finally {
      if (guard.isCurrent(requestSequence)) loading.value = false
    }
  }

  function sendSubscription(type: Request_Type, serverId: string): boolean {
    if (serverId === '') return false
    const websocket = GetOrCreateXylonaWebsocketClient()
    if (!websocket.isOpen()) return false
    const request = create(RequestSchema, { gameServerId: serverId, type })
    websocket.send(toJsonString(RequestSchema, request))
    return true
  }

  function appendLiveMetrics(metrics: GameServerMetrics): void {
    liveProcessMetrics.value = metrics
    const currentRange = getMetricsRangeRequest(selectedRange.value)
    if (!currentRange.live) return
    const nowMs = Date.now()
    const bucketMs = Math.max(
      Math.floor((currentRange.untilMs - currentRange.sinceMs) / currentRange.maxPoints),
      3000,
    )
    const collectedAtMs = timestampToMs(metrics.collectedAt) ?? nowMs
    const timestampMs = Math.floor(collectedAtMs / bucketMs) * bucketMs
    const configuredMemoryBytes =
      Number(gameServer.value.maxMemoryMb) > 0
        ? Number(gameServer.value.maxMemoryMb) * 1024 * 1024
        : null
    const merged = mergeMetricSamples(
      samples.value,
      normalizeLiveMetricPoint(metrics, {
        timestampMs,
        collectedAtMs,
        latest: latestSample.value,
        nodeMemoryUsedBytes: nodeMemoryUsedBytes.value,
        nodeMemoryTotalBytes: nodeMemoryTotalBytes.value,
        configuredMemoryBytes,
        nodeId: gameServer.value.nodeId,
      }),
      currentRange.maxPoints,
    )
    const rangeDurationMs = currentRange.untilMs - currentRange.sinceMs
    samples.value = merged.filter((sample) => sample.timestampMs >= nowMs - rangeDurationMs)
  }

  function onServerMetrics(metrics: AllServersMetrics): void {
    const serverMetrics = metrics.servers[gameServerId.value]
    if (serverMetrics) appendLiveMetrics(serverMetrics)
  }

  function onNodeMetrics(metrics: AllNodeMetrics): void {
    const nodeMetrics = metrics.nodes[gameServer.value.nodeId]
    if (!nodeMetrics) return
    nodeMemoryUsedBytes.value = Number(nodeMetrics.memoryUsedBytes)
    nodeMemoryTotalBytes.value = Number(nodeMetrics.memoryTotalBytes)
    if (liveProcessMetrics.value && rangeRequest.value.live)
      appendLiveMetrics(liveProcessMetrics.value)
  }

  function updateMetricsSubscription(nextServerId: string): void {
    const transition = getMetricsSubscriptionTransition(subscribedGameServerId, nextServerId)
    if (transition.unsubscribeId !== null) {
      sendSubscription(Request_Type.UnsubscribeServerMetrics, transition.unsubscribeId)
      subscribedGameServerId = ''
    }
    if (
      transition.subscribeId !== null &&
      sendSubscription(Request_Type.SubscribeServerMetrics, transition.subscribeId)
    ) {
      subscribedGameServerId = transition.subscribeId
    }
  }

  function resubscribeCurrentServer(): void {
    subscribedGameServerId = ''
    updateMetricsSubscription(gameServerId.value)
  }

  watch(
    gameServerId,
    (nextServerId) => {
      resetServerScopedState()
      if (subscriptionMounted) {
        updateMetricsSubscription('')
        void nextTick(() => {
          if (subscriptionMounted && gameServerId.value === nextServerId) {
            updateMetricsSubscription(nextServerId)
          }
        })
      }
      void fetchMetrics()
    },
    { flush: 'sync', immediate: true },
  )
  watch(selectedRange, () => void fetchMetrics())

  onMounted(() => {
    subscriptionMounted = true
    XylonaEventBus.on('gameServerMetrics', onServerMetrics)
    XylonaEventBus.on('nodeMetrics', onNodeMetrics)
    XylonaEventBus.on('websocketConnected', resubscribeCurrentServer)
    updateMetricsSubscription(gameServerId.value)
    historyRefreshTimer = setInterval(() => {
      if (rangeRequest.value.live) void fetchMetrics()
    }, 60_000)
  })

  onBeforeUnmount(() => {
    subscriptionMounted = false
    guard.invalidate()
    XylonaEventBus.off('gameServerMetrics', onServerMetrics)
    XylonaEventBus.off('nodeMetrics', onNodeMetrics)
    XylonaEventBus.off('websocketConnected', resubscribeCurrentServer)
    if (historyRefreshTimer !== undefined) clearInterval(historyRefreshTimer)
    updateMetricsSubscription('')
  })

  return {
    currentCapacity,
    error,
    fetchMetrics,
    gameServer,
    latestSample,
    loading,
    mixedResolution,
    resolution,
    sampleIntervalSeconds,
    samples,
    selectedRange,
    timeline,
    viewState,
  }
}
