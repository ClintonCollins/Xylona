export type MetricsRangeKey = '1h' | '6h' | '24h' | '7d' | '30d' | '90d'

export interface MetricsRangeOption {
  label: string
  value: MetricsRangeKey
  durationMs: number
  maxPoints: number
  live: boolean
}

const defaultMetricsRangeOption: MetricsRangeOption = {
  label: '1h',
  value: '1h',
  durationMs: 60 * 60 * 1000,
  maxPoints: 240,
  live: true,
}

export const metricsRangeOptions: MetricsRangeOption[] = [
  defaultMetricsRangeOption,
  { label: '6h', value: '6h', durationMs: 6 * 60 * 60 * 1000, maxPoints: 360, live: true },
  { label: '24h', value: '24h', durationMs: 24 * 60 * 60 * 1000, maxPoints: 480, live: true },
  { label: '7d', value: '7d', durationMs: 7 * 24 * 60 * 60 * 1000, maxPoints: 504, live: false },
  { label: '30d', value: '30d', durationMs: 30 * 24 * 60 * 60 * 1000, maxPoints: 720, live: false },
  {
    label: '90d',
    value: '90d',
    durationMs: 90 * 24 * 60 * 60 * 1000,
    maxPoints: 1080,
    live: false,
  },
]

export interface MetricsRangeRequest {
  sinceMs: number
  untilMs: number
  maxPoints: number
  live: boolean
}

export function getMetricsRangeOption(range: MetricsRangeKey): MetricsRangeOption {
  return (
    metricsRangeOptions.find((candidate) => candidate.value === range) ?? defaultMetricsRangeOption
  )
}

export function getMetricsRangeRequest(
  range: MetricsRangeKey,
  nowMs = Date.now(),
): MetricsRangeRequest {
  const resolved = getMetricsRangeOption(range)

  return {
    sinceMs: nowMs - resolved.durationMs,
    untilMs: nowMs,
    maxPoints: resolved.maxPoints,
    live: resolved.live,
  }
}

export class LatestRequestGuard {
  private sequence = 0

  begin(): number {
    this.sequence += 1
    return this.sequence
  }

  isCurrent(requestSequence: number): boolean {
    return requestSequence === this.sequence
  }

  invalidate(): void {
    this.sequence += 1
  }
}

export interface MetricsSubscriptionTransition {
  unsubscribeId: string | null
  subscribeId: string | null
}

export function getMetricsSubscriptionTransition(
  subscribedServerId: string,
  nextServerId: string,
): MetricsSubscriptionTransition {
  if (subscribedServerId === nextServerId) {
    return { unsubscribeId: null, subscribeId: null }
  }

  return {
    unsubscribeId: subscribedServerId === '' ? null : subscribedServerId,
    subscribeId: nextServerId === '' ? null : nextServerId,
  }
}

export const metricsCollectionStatus = {
  unspecified: 0,
  available: 1,
  warmingUp: 2,
  serverOffline: 3,
  nodeUnavailable: 4,
  collectorError: 5,
} as const

export interface MetricSample {
  timestampMs: number
  cpuAverage: number | null
  cpuMinimum: number | null
  cpuMaximum: number | null
  nodeCpuCores: number | null
  memoryRssAverage: number | null
  memoryRssMinimum: number | null
  memoryRssMaximum: number | null
  memoryPercentAverage: number | null
  memoryPercentMinimum: number | null
  memoryPercentMaximum: number | null
  nodeMemoryUsedBytes: number | null
  nodeMemoryTotalBytes: number | null
  configuredMemoryBytes: number | null
  diskUsageAverage: number | null
  diskUsageMinimum: number | null
  diskUsageMaximum: number | null
  volumeTotalBytes: number | null
  volumeFreeBytes: number | null
  volumePercent: number | null
  volumeValid: boolean
  ioValid: boolean
  connectionCountValid: boolean
  diskMeasuredAtMs: number | null
  ioReadAverage: number | null
  ioReadMinimum: number | null
  ioReadMaximum: number | null
  ioWriteAverage: number | null
  ioWriteMinimum: number | null
  ioWriteMaximum: number | null
  connectionAverage: number | null
  connectionMinimum: number | null
  connectionMaximum: number | null
  playerAverage: number | null
  playerMinimum: number | null
  playerMaximum: number | null
  playerCapacity: number | null
  querySupported: boolean | null
  querySuccess: boolean | null
  queryDurationAverage: number | null
  queryDurationMinimum: number | null
  queryDurationMaximum: number | null
  queryCheckedAtMs: number | null
  serverFpsAverage: number | null
  serverFpsMinimum: number | null
  serverFpsMaximum: number | null
  frameTimeAverage: number | null
  frameTimeMinimum: number | null
  frameTimeMaximum: number | null
  uptimeSeconds: number | null
  processStatus: string
  collectionStatus: number
  processCollectedAtMs: number | null
  granularitySeconds: number
  sampleCount: number
  availableSampleCount: number
  cpuValidSampleCount: number
  volumeValidSampleCount: number
  ioValidSampleCount: number
  connectionValidSampleCount: number
  querySuccessfulSampleCount: number
  queryDurationValidSampleCount: number
  serverFpsValidSampleCount: number
  serverFrameTimeValidSampleCount: number
  availabilityRatio: number
  nodeId: string
  source: 'history' | 'live'
}

export interface MetricCapacity {
  processRssBytes: number | null
  configuredTargetBytes: number | null
  configuredTargetRatio: number | null
  nodeUsedBytes: number | null
  nodeAvailableBytes: number | null
  nodeTotalBytes: number | null
  nodeProcessShareRatio: number | null
}

export function calculateMetricCapacity(input: {
  processRssBytes: number | null
  configuredTargetBytes: number | null
  nodeUsedBytes: number | null
  nodeTotalBytes: number | null
}): MetricCapacity {
  const configuredTargetRatio =
    input.processRssBytes !== null &&
    input.configuredTargetBytes !== null &&
    input.configuredTargetBytes > 0
      ? input.processRssBytes / input.configuredTargetBytes
      : null
  const nodeAvailableBytes =
    input.nodeUsedBytes !== null && input.nodeTotalBytes !== null
      ? Math.max(input.nodeTotalBytes - input.nodeUsedBytes, 0)
      : null
  const nodeProcessShareRatio =
    input.processRssBytes !== null && input.nodeTotalBytes !== null && input.nodeTotalBytes > 0
      ? input.processRssBytes / input.nodeTotalBytes
      : null

  return {
    processRssBytes: input.processRssBytes,
    configuredTargetBytes: input.configuredTargetBytes,
    configuredTargetRatio,
    nodeUsedBytes: input.nodeUsedBytes,
    nodeAvailableBytes,
    nodeTotalBytes: input.nodeTotalBytes,
    nodeProcessShareRatio,
  }
}

type CompleteVolumeCapacitySample = MetricSample & {
  volumeValid: true
  volumeTotalBytes: number
  volumeFreeBytes: number
  volumePercent: number
  diskMeasuredAtMs: number
}

export function hasCompleteVolumeCapacity(
  sample: MetricSample | null,
): sample is CompleteVolumeCapacitySample {
  return (
    sample !== null &&
    sample.volumeValid &&
    sample.volumeTotalBytes !== null &&
    sample.volumeFreeBytes !== null &&
    sample.volumePercent !== null &&
    sample.diskMeasuredAtMs !== null
  )
}

export type MetricHealthLevel = 'ok' | 'warn' | 'danger' | 'unknown'

export interface MetricHealth {
  level: MetricHealthLevel
  label: string
}

export interface ServerHealthAttentionItem {
  key: 'cpu' | 'memory' | 'volume' | 'query'
  level: 'warn' | 'danger'
  label: string
}

export interface ServerHealth {
  cpu: MetricHealth
  memory: MetricHealth
  volume: MetricHealth
  query: MetricHealth
  attention: ServerHealthAttentionItem[]
  nominalCount: number
}

const cpuWarnPercent = 85
const cpuDangerPercent = 95
const memoryWarnRatio = 0.85
const memoryDangerRatio = 0.95
const volumeWarnPercent = 80
const volumeDangerPercent = 92

export function deriveServerHealth(input: {
  latestSample: MetricSample | null
  capacity: MetricCapacity
}): ServerHealth {
  const latest = input.latestSample

  let cpu: MetricHealth = { level: 'unknown', label: 'Unknown' }
  const cpuPercent = latest?.cpuAverage ?? null
  if (cpuPercent !== null) {
    if (cpuPercent >= cpuDangerPercent) cpu = { level: 'danger', label: 'Saturated' }
    else if (cpuPercent >= cpuWarnPercent) cpu = { level: 'warn', label: 'High load' }
    else cpu = { level: 'ok', label: 'Nominal' }
  }

  let memory: MetricHealth = { level: 'unknown', label: 'Unknown' }
  const targetRatio = input.capacity.configuredTargetRatio
  if (targetRatio !== null) {
    const targetPercent = `${Math.round(targetRatio * 100)}% of target`
    if (targetRatio >= memoryDangerRatio) memory = { level: 'danger', label: targetPercent }
    else if (targetRatio >= memoryWarnRatio) memory = { level: 'warn', label: targetPercent }
    else memory = { level: 'ok', label: 'Nominal' }
  } else if (input.capacity.processRssBytes !== null) {
    memory = { level: 'ok', label: 'No target set' }
  }

  let volume: MetricHealth = { level: 'unknown', label: 'Unavailable' }
  if (hasCompleteVolumeCapacity(latest)) {
    if (latest.volumePercent >= volumeDangerPercent)
      volume = { level: 'danger', label: 'Low space' }
    else if (latest.volumePercent >= volumeWarnPercent)
      volume = { level: 'warn', label: 'Filling up' }
    else volume = { level: 'ok', label: 'Nominal' }
  }

  let query: MetricHealth = { level: 'unknown', label: 'Unknown' }
  if (latest?.querySupported === false) query = { level: 'unknown', label: 'Not supported' }
  else if (latest?.querySuccess === false) query = { level: 'warn', label: 'Query failing' }
  else if (latest?.querySuccess === true) query = { level: 'ok', label: 'Healthy' }

  const attention: ServerHealthAttentionItem[] = []
  if (cpu.level === 'warn' || cpu.level === 'danger') {
    attention.push({
      key: 'cpu',
      level: cpu.level,
      label:
        `CPU ${cpuPercent === null ? '' : `${cpuPercent.toFixed(0)}% `}${cpu.label.toLowerCase()}`.trim(),
    })
  }
  if (memory.level === 'warn' || memory.level === 'danger') {
    attention.push({ key: 'memory', level: memory.level, label: `Memory ${memory.label}` })
  }
  if (volume.level === 'warn' || volume.level === 'danger') {
    const usedPercent = hasCompleteVolumeCapacity(latest)
      ? ` ${latest.volumePercent.toFixed(0)}% used`
      : ''
    attention.push({ key: 'volume', level: volume.level, label: `Volume${usedPercent}` })
  }
  if (query.level === 'warn' || query.level === 'danger') {
    attention.push({ key: 'query', level: query.level, label: 'Game query failing' })
  }
  attention.sort((left, right) =>
    left.level === right.level ? 0 : left.level === 'danger' ? -1 : 1,
  )

  const nominalCount = [cpu, memory, volume, query].filter((health) => health.level === 'ok').length

  return { cpu, memory, volume, query, attention, nominalCount }
}

export function isMetricsRangeKey(value: unknown): value is MetricsRangeKey {
  return (
    typeof value === 'string' && metricsRangeOptions.some((candidate) => candidate.value === value)
  )
}

export type MetricsViewStateKind =
  | 'loading'
  | 'error'
  | 'no-data'
  | 'available'
  | 'stale'
  | 'warming-up'
  | 'offline'
  | 'node-unavailable'
  | 'collector-error'
  | 'unknown'

export interface MetricsViewState {
  kind: MetricsViewStateKind
  label: string
  detail: string
}

export function deriveMetricsViewState(input: {
  loading: boolean
  error: string
  samples: readonly MetricSample[]
  nowMs?: number
  websocketConnected?: boolean
}): MetricsViewState {
  if (input.loading && input.samples.length === 0) {
    return { kind: 'loading', label: 'Loading', detail: 'Loading recorded server telemetry.' }
  }
  if (input.error !== '') {
    return { kind: 'error', label: 'Unavailable', detail: input.error }
  }
  const latest = input.samples[input.samples.length - 1]
  if (!latest) {
    return {
      kind: 'no-data',
      label: 'No data',
      detail: 'No telemetry was recorded in this time range.',
    }
  }

  if (latest.collectionStatus === metricsCollectionStatus.warmingUp) {
    return {
      kind: 'warming-up',
      label: 'Warming up',
      detail: 'The collector needs another interval before rates are trustworthy.',
    }
  }
  if (latest.collectionStatus === metricsCollectionStatus.serverOffline) {
    return { kind: 'offline', label: 'Server offline', detail: 'The server was not running.' }
  }
  if (latest.collectionStatus === metricsCollectionStatus.nodeUnavailable) {
    return {
      kind: 'node-unavailable',
      label: 'Node unavailable',
      detail: 'The controller could not reach the assigned node.',
    }
  }
  if (latest.collectionStatus === metricsCollectionStatus.collectorError) {
    return {
      kind: 'collector-error',
      label: 'Collection failed',
      detail: 'The node reported a telemetry collection error.',
    }
  }
  if (latest.collectionStatus !== metricsCollectionStatus.available) {
    return {
      kind: 'unknown',
      label: 'Unknown',
      detail: 'The latest telemetry sample has no confirmed availability state.',
    }
  }

  if (input.websocketConnected === false && latest.source === 'live') {
    return {
      kind: 'stale',
      label: 'Reconnecting',
      detail: 'Live telemetry is paused while the controller connection reconnects.',
    }
  }

  const nowMs = input.nowMs ?? Date.now()
  const collectedAt = latest.processCollectedAtMs ?? latest.timestampMs
  const staleAfterMs = Math.max(latest.granularitySeconds * 3 * 1000, 2 * 60 * 1000)
  if (nowMs - collectedAt > staleAfterMs) {
    return {
      kind: 'stale',
      label: 'Stale',
      detail: 'The latest sample is older than the expected collection interval.',
    }
  }

  return { kind: 'available', label: 'Live', detail: 'Telemetry is current.' }
}

export function mergeMetricSamples(
  recorded: readonly MetricSample[],
  incoming: MetricSample,
  maxPoints: number,
): MetricSample[] {
  const byTimestamp = new Map(recorded.map((sample) => [sample.timestampMs, sample]))
  byTimestamp.set(incoming.timestampMs, incoming)
  const merged = [...byTimestamp.values()].sort(
    (left, right) => left.timestampMs - right.timestampMs,
  )
  return merged.slice(Math.max(merged.length - maxPoints, 0))
}

export function mergeMetricHistoryWithLiveTail(
  history: readonly MetricSample[],
  current: readonly MetricSample[],
  sinceMs: number,
  maxPoints: number,
): MetricSample[] {
  const newestHistoryTimestamp = history.reduce(
    (latest, sample) => Math.max(latest, sample.timestampMs),
    Number.NEGATIVE_INFINITY,
  )
  let merged = [...history]
  for (const liveSample of current) {
    if (
      liveSample.source !== 'live' ||
      liveSample.timestampMs < sinceMs ||
      liveSample.timestampMs <= newestHistoryTimestamp
    ) {
      continue
    }
    merged = mergeMetricSamples(merged, liveSample, maxPoints)
  }
  return merged
}

export interface MetricSummary {
  latest: number | null
  minimum: number | null
  average: number | null
  maximum: number | null
  coverageRatio: number | null
  sampleCount: number
}

export function summarizeMetric(
  samples: readonly MetricSample[],
  selectors: {
    value: (sample: MetricSample) => number | null
    minimum: (sample: MetricSample) => number | null
    maximum: (sample: MetricSample) => number | null
    validSampleCount?: (sample: MetricSample) => number
  },
): MetricSummary {
  let weightedTotal = 0
  let weight = 0
  let minimum: number | null = null
  let maximum: number | null = null
  let metricAvailableSamples = 0
  let sampleCount = 0

  for (const sample of samples) {
    sampleCount += sample.sampleCount
    const selectedValidSampleCount =
      selectors.validSampleCount?.(sample) ?? sample.availableSampleCount
    const validSampleCount = Math.min(
      Math.max(selectedValidSampleCount, 0),
      Math.max(sample.sampleCount, 0),
    )
    if (validSampleCount === 0) continue

    const value = selectors.value(sample)
    if (value !== null && Number.isFinite(value)) {
      weightedTotal += value * validSampleCount
      weight += validSampleCount
      metricAvailableSamples += validSampleCount
    }
    const sampleMinimum = selectors.minimum(sample)
    if (sampleMinimum !== null && Number.isFinite(sampleMinimum)) {
      minimum = minimum === null ? sampleMinimum : Math.min(minimum, sampleMinimum)
    }
    const sampleMaximum = selectors.maximum(sample)
    if (sampleMaximum !== null && Number.isFinite(sampleMaximum)) {
      maximum = maximum === null ? sampleMaximum : Math.max(maximum, sampleMaximum)
    }
  }

  const latestSample = [...samples].reverse().find((sample) => {
    const selectedValidSampleCount =
      selectors.validSampleCount?.(sample) ?? sample.availableSampleCount
    return selectedValidSampleCount > 0 && selectors.value(sample) !== null
  })

  return {
    latest: latestSample ? selectors.value(latestSample) : null,
    minimum,
    average: weight > 0 ? weightedTotal / weight : null,
    maximum,
    coverageRatio: sampleCount > 0 ? Math.min(metricAvailableSamples / sampleCount, 1) : null,
    sampleCount,
  }
}
