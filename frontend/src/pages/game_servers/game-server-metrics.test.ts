import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { describe, expect, it } from 'vitest'
import { GameServerMetricsHistoryPointSchema } from '@/proto/shared_pb'
import { GameServerMetricsSchema } from '@/proto/websocket_pb'
import {
  calculateMetricCapacity,
  deriveMetricsViewState,
  getMetricsRangeRequest,
  getMetricsSubscriptionTransition,
  hasCompleteVolumeCapacity,
  LatestRequestGuard,
  metricsCollectionStatus,
  mergeMetricHistoryWithLiveTail,
  summarizeMetric,
  type MetricSample,
} from './game-server-metrics'
import { normalizeLiveMetricPoint, normalizeMetricPoint } from './useGameServerMetrics'

function sample(overrides: Partial<MetricSample> = {}): MetricSample {
  return {
    timestampMs: 1_000_000,
    cpuAverage: 25,
    cpuMinimum: 20,
    cpuMaximum: 30,
    nodeCpuCores: 8,
    memoryRssAverage: 1024,
    memoryRssMinimum: 900,
    memoryRssMaximum: 1100,
    memoryPercentAverage: 1,
    memoryPercentMinimum: 0.9,
    memoryPercentMaximum: 1.1,
    nodeMemoryUsedBytes: 4096,
    nodeMemoryTotalBytes: 8192,
    configuredMemoryBytes: 2048,
    diskUsageAverage: 1024,
    diskUsageMinimum: 900,
    diskUsageMaximum: 1100,
    volumeTotalBytes: 8192,
    volumeFreeBytes: 4096,
    volumePercent: 50,
    volumeValid: true,
    ioValid: true,
    connectionCountValid: true,
    diskMeasuredAtMs: 1_000_000,
    ioReadAverage: 0,
    ioReadMinimum: 0,
    ioReadMaximum: 0,
    ioWriteAverage: 0,
    ioWriteMinimum: 0,
    ioWriteMaximum: 0,
    connectionAverage: 0,
    connectionMinimum: 0,
    connectionMaximum: 0,
    playerAverage: 0,
    playerMinimum: 0,
    playerMaximum: 0,
    playerCapacity: 10,
    querySupported: true,
    querySuccess: true,
    queryDurationAverage: 4,
    queryDurationMinimum: 3,
    queryDurationMaximum: 5,
    queryCheckedAtMs: 1_000_000,
    serverFpsAverage: null,
    serverFpsMinimum: null,
    serverFpsMaximum: null,
    frameTimeAverage: null,
    frameTimeMinimum: null,
    frameTimeMaximum: null,
    uptimeSeconds: 60,
    processStatus: 'online',
    collectionStatus: metricsCollectionStatus.available,
    processCollectedAtMs: 1_000_000,
    granularitySeconds: 60,
    sampleCount: 1,
    availableSampleCount: 1,
    cpuValidSampleCount: 1,
    volumeValidSampleCount: 1,
    ioValidSampleCount: 1,
    connectionValidSampleCount: 1,
    querySuccessfulSampleCount: 1,
    queryDurationValidSampleCount: 1,
    serverFpsValidSampleCount: 0,
    serverFrameTimeValidSampleCount: 0,
    availabilityRatio: 1,
    nodeId: 'node-1',
    source: 'history',
    ...overrides,
  }
}

describe('game server metrics helpers', () => {
  it.each([
    ['1h', 60 * 60 * 1000, 240, true],
    ['6h', 6 * 60 * 60 * 1000, 360, true],
    ['24h', 24 * 60 * 60 * 1000, 480, true],
    ['7d', 7 * 24 * 60 * 60 * 1000, 504, false],
    ['30d', 30 * 24 * 60 * 60 * 1000, 720, false],
    ['90d', 90 * 24 * 60 * 60 * 1000, 1080, false],
  ] as const)('maps %s to its query window and point cap', (range, duration, maxPoints, live) => {
    const request = getMetricsRangeRequest(range, 100_000_000)

    expect(request).toEqual({
      sinceMs: 100_000_000 - duration,
      untilMs: 100_000_000,
      maxPoints,
      live,
    })
  })

  it('suppresses responses that are no longer current', () => {
    const guard = new LatestRequestGuard()
    const first = guard.begin()
    const second = guard.begin()

    expect(guard.isCurrent(first)).toBe(false)
    expect(guard.isCurrent(second)).toBe(true)
    guard.invalidate()
    expect(guard.isCurrent(second)).toBe(false)
  })

  it.each([
    ['server-old', 'server-new', 'server-old', 'server-new'],
    ['server-old', '', 'server-old', null],
    ['', 'server-new', null, 'server-new'],
    ['server-same', 'server-same', null, null],
  ] as const)(
    'uses the captured subscription ID when moving from %s to %s',
    (subscribedServerId, nextServerId, unsubscribeId, subscribeId) => {
      expect(getMetricsSubscriptionTransition(subscribedServerId, nextServerId)).toEqual({
        unsubscribeId,
        subscribeId,
      })
    },
  )

  it('calculates target pressure separately from node available memory', () => {
    const capacity = calculateMetricCapacity({
      processRssBytes: 2_000,
      configuredTargetBytes: 4_000,
      nodeUsedBytes: 6_000,
      nodeTotalBytes: 10_000,
    })

    expect(capacity.configuredTargetRatio).toBe(0.5)
    expect(capacity.nodeAvailableBytes).toBe(4_000)
    expect(capacity.nodeProcessShareRatio).toBe(0.2)
  })

  it.each([
    ['volume total', { volumeTotalBytes: null }],
    ['volume free', { volumeFreeBytes: null }],
    ['volume percent', { volumePercent: null }],
    ['measurement timestamp', { diskMeasuredAtMs: null }],
  ] as const)('requires %s before showing volume capacity', (_label, overrides) => {
    expect(hasCompleteVolumeCapacity(sample(overrides))).toBe(false)
  })

  it('accepts a complete valid volume capacity sample', () => {
    expect(hasCompleteVolumeCapacity(sample())).toBe(true)
  })

  it.each([
    [metricsCollectionStatus.warmingUp, 'warming-up'],
    [metricsCollectionStatus.serverOffline, 'offline'],
    [metricsCollectionStatus.nodeUnavailable, 'node-unavailable'],
    [metricsCollectionStatus.collectorError, 'collector-error'],
    [metricsCollectionStatus.unspecified, 'unknown'],
  ] as const)('maps collection status %i to %s', (collectionStatus, expected) => {
    expect(
      deriveMetricsViewState({
        loading: false,
        error: '',
        samples: [sample({ collectionStatus })],
        nowMs: 1_000_001,
      }).kind,
    ).toBe(expected)
  })

  it('distinguishes current, stale, empty, and request-error states', () => {
    expect(
      deriveMetricsViewState({ loading: false, error: '', samples: [], nowMs: 1_000_001 }).kind,
    ).toBe('no-data')
    expect(
      deriveMetricsViewState({
        loading: false,
        error: '',
        samples: [sample()],
        nowMs: 1_000_001,
      }).kind,
    ).toBe('available')
    expect(
      deriveMetricsViewState({
        loading: false,
        error: '',
        samples: [sample()],
        nowMs: 1_200_001,
      }).kind,
    ).toBe('stale')
    expect(
      deriveMetricsViewState({
        loading: false,
        error: 'Request failed',
        samples: [sample()],
        nowMs: 1_000_001,
      }).kind,
    ).toBe('error')
  })

  it('reports aggregate extrema and metric-specific sample coverage', () => {
    const summary = summarizeMetric(
      [
        sample({
          cpuAverage: 25,
          cpuMinimum: 10,
          cpuMaximum: 40,
          sampleCount: 4,
          availableSampleCount: 0,
          cpuValidSampleCount: 3,
        }),
        sample({
          cpuAverage: null,
          cpuMinimum: null,
          cpuMaximum: null,
          sampleCount: 2,
          availableSampleCount: 2,
          cpuValidSampleCount: 0,
        }),
      ],
      {
        value: (metric) => metric.cpuAverage,
        minimum: (metric) => metric.cpuMinimum,
        maximum: (metric) => metric.cpuMaximum,
        validSampleCount: (metric) => metric.cpuValidSampleCount,
      },
    )

    expect(summary).toMatchObject({ minimum: 10, average: 25, maximum: 40, coverageRatio: 0.5 })
  })

  it('weights rollup averages by metric-specific valid samples', () => {
    const summary = summarizeMetric(
      [
        sample({ cpuAverage: 10, sampleCount: 2, cpuValidSampleCount: 1 }),
        sample({ cpuAverage: 20, sampleCount: 3, cpuValidSampleCount: 3 }),
      ],
      {
        value: (metric) => metric.cpuAverage,
        minimum: (metric) => metric.cpuMinimum,
        maximum: (metric) => metric.cpuMaximum,
        validSampleCount: (metric) => metric.cpuValidSampleCount,
      },
    )

    expect(summary).toMatchObject({ average: 17.5, coverageRatio: 0.8, sampleCount: 5 })
  })

  it('keeps valid zero I/O and connection values while turning invalid values into gaps', () => {
    const timestamp = create(TimestampSchema, { seconds: 1000n })
    const valid = normalizeMetricPoint(
      create(GameServerMetricsHistoryPointSchema, {
        timestamp,
        collectionStatus: metricsCollectionStatus.available,
        sampleCount: 1,
        availableSampleCount: 1,
        ioValid: true,
        ioValidSampleCount: 1,
        ioReadRate: 0,
        ioWriteRate: 0,
        connectionCountValid: true,
        connectionValidSampleCount: 1,
        connectionCount: 0,
      }),
    )
    const invalid = normalizeMetricPoint(
      create(GameServerMetricsHistoryPointSchema, {
        timestamp,
        collectionStatus: metricsCollectionStatus.available,
        sampleCount: 1,
        availableSampleCount: 1,
        ioValid: false,
        ioValidSampleCount: 0,
        ioReadRate: 128,
        ioWriteRate: 256,
        connectionCountValid: false,
        connectionValidSampleCount: 0,
        connectionCount: 4,
      }),
    )

    expect(valid).toMatchObject({
      ioReadAverage: 0,
      ioWriteAverage: 0,
      connectionAverage: 0,
    })
    expect(invalid).toMatchObject({
      ioReadAverage: null,
      ioWriteAverage: null,
      connectionAverage: null,
    })
  })

  it('preserves independently valid query metrics and gaps missing capabilities', () => {
    const normalized = normalizeMetricPoint(
      create(GameServerMetricsHistoryPointSchema, {
        timestamp: create(TimestampSchema, { seconds: 1000n }),
        collectionStatus: metricsCollectionStatus.available,
        sampleCount: 4,
        availableSampleCount: 4,
        querySuccessfulSampleCount: 4,
        queryDurationValidSampleCount: 2,
        queryDurationMs: 12,
        queryDurationMsMin: 10,
        queryDurationMsMax: 14,
        serverFpsValidSampleCount: 0,
        serverFps: 60,
        serverFrameTimeValidSampleCount: 1,
        serverFrameTimeMs: 16.7,
        serverFrameTimeMsMin: 16,
        serverFrameTimeMsMax: 18,
      }),
    )

    expect(normalized).toMatchObject({
      querySuccessfulSampleCount: 4,
      queryDurationValidSampleCount: 2,
      queryDurationAverage: 12,
      queryDurationMinimum: 10,
      queryDurationMaximum: 14,
      serverFpsValidSampleCount: 0,
      serverFpsAverage: null,
      serverFpsMinimum: null,
      serverFpsMaximum: null,
      serverFrameTimeValidSampleCount: 1,
      frameTimeAverage: 16.7,
      frameTimeMinimum: 16,
      frameTimeMaximum: 18,
    })
  })

  it('weights query metric summaries by each field-specific validity count', () => {
    const queryDurationSummary = summarizeMetric(
      [
        sample({
          queryDurationAverage: 10,
          sampleCount: 4,
          querySuccessfulSampleCount: 4,
          queryDurationValidSampleCount: 1,
        }),
        sample({
          queryDurationAverage: 30,
          sampleCount: 4,
          querySuccessfulSampleCount: 4,
          queryDurationValidSampleCount: 3,
        }),
      ],
      {
        value: (metric) => metric.queryDurationAverage,
        minimum: (metric) => metric.queryDurationMinimum,
        maximum: (metric) => metric.queryDurationMaximum,
        validSampleCount: (metric) => metric.queryDurationValidSampleCount,
      },
    )

    expect(queryDurationSummary).toMatchObject({ average: 25, coverageRatio: 0.5 })
  })

  it('summarizes frame-time extrema and coverage using frame-time-valid samples', () => {
    const frameTimeSummary = summarizeMetric(
      [
        sample({
          frameTimeAverage: 10,
          frameTimeMinimum: 8,
          frameTimeMaximum: 12,
          sampleCount: 4,
          serverFrameTimeValidSampleCount: 1,
        }),
        sample({
          timestampMs: 2_000_000,
          frameTimeAverage: 20,
          frameTimeMinimum: 17,
          frameTimeMaximum: 25,
          sampleCount: 4,
          serverFrameTimeValidSampleCount: 3,
        }),
      ],
      {
        value: (metric) => metric.frameTimeAverage,
        minimum: (metric) => metric.frameTimeMinimum,
        maximum: (metric) => metric.frameTimeMaximum,
        validSampleCount: (metric) => metric.serverFrameTimeValidSampleCount,
      },
    )

    expect(frameTimeSummary).toMatchObject({
      latest: 20,
      minimum: 8,
      average: 17.5,
      maximum: 25,
      coverageRatio: 0.5,
      sampleCount: 8,
    })
  })

  it('does not copy stale query telemetry into a live process sample', () => {
    const normalized = normalizeLiveMetricPoint(
      create(GameServerMetricsSchema, {
        metricsValid: true,
        processStatus: 'online',
        collectionStatus: metricsCollectionStatus.available,
      }),
      {
        timestampMs: 2_000_000,
        collectedAtMs: 2_000_000,
        latest: sample(),
        nodeMemoryUsedBytes: null,
        nodeMemoryTotalBytes: null,
        configuredMemoryBytes: null,
        nodeId: 'node-1',
      },
    )

    expect(normalized).toMatchObject({
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
      querySuccessfulSampleCount: 0,
      queryDurationValidSampleCount: 0,
      serverFpsValidSampleCount: 0,
      serverFrameTimeValidSampleCount: 0,
    })
  })

  it('uses the actual live collection time when evaluating freshness', () => {
    const bucketStartMs = 180_000
    const collectedAtMs = 359_000
    const normalized = normalizeLiveMetricPoint(
      create(GameServerMetricsSchema, {
        metricsValid: true,
        processStatus: 'online',
        collectionStatus: metricsCollectionStatus.available,
      }),
      {
        timestampMs: bucketStartMs,
        collectedAtMs,
        latest: null,
        nodeMemoryUsedBytes: null,
        nodeMemoryTotalBytes: null,
        configuredMemoryBytes: null,
        nodeId: 'node-1',
      },
    )

    expect(normalized.timestampMs).toBe(bucketStartMs)
    expect(normalized.processCollectedAtMs).toBe(collectedAtMs)
    expect(
      deriveMetricsViewState({
        loading: false,
        error: '',
        samples: [normalized],
        websocketConnected: true,
        nowMs: collectedAtMs + 1,
      }).kind,
    ).toBe('available')
  })

  it('preserves the WebSocket offline process and collection status', () => {
    const normalized = normalizeLiveMetricPoint(
      create(GameServerMetricsSchema, {
        metricsValid: false,
        processStatus: 'offline',
        collectionStatus: metricsCollectionStatus.serverOffline,
      }),
      {
        timestampMs: 2_000_000,
        collectedAtMs: 2_000_000,
        latest: sample(),
        nodeMemoryUsedBytes: null,
        nodeMemoryTotalBytes: null,
        configuredMemoryBytes: null,
        nodeId: 'node-1',
      },
    )

    expect(normalized).toMatchObject({
      processStatus: 'offline',
      collectionStatus: metricsCollectionStatus.serverOffline,
      memoryRssAverage: null,
      availableSampleCount: 0,
      availabilityRatio: 0,
    })
  })

  it('preserves only the live tail newer than refreshed history', () => {
    const refreshed = mergeMetricHistoryWithLiveTail(
      [
        sample({ timestampMs: 1_000, source: 'history' }),
        sample({ timestampMs: 2_000, source: 'history', cpuAverage: 38 }),
      ],
      [
        sample({ timestampMs: 500, source: 'live' }),
        sample({ timestampMs: 1_500, source: 'live', cpuAverage: 40 }),
        sample({ timestampMs: 2_000, source: 'live', cpuAverage: 42 }),
        sample({ timestampMs: 2_500, source: 'live', cpuAverage: 44 }),
        sample({ timestampMs: 3_000, source: 'history' }),
      ],
      1_000,
      10,
    )

    expect(refreshed.map((metric) => [metric.timestampMs, metric.source])).toEqual([
      [1_000, 'history'],
      [2_000, 'history'],
      [2_500, 'live'],
    ])
    expect(refreshed[1]?.cpuAverage).toBe(38)
    expect(refreshed[2]?.cpuAverage).toBe(44)
  })
})
