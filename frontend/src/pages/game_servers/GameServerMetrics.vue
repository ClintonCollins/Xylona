<template>
  <main class="metrics-page xy-page-content">
    <header class="metrics-page__header">
      <div>
        <div class="metrics-page__title-row">
          <q-icon aria-hidden="true" color="primary" name="insights" size="24px" />
          <h1>Performance & health</h1>
          <span :class="`metrics-state--${viewState.kind}`" class="metrics-state">
            {{ viewState.label }}
          </span>
        </div>
        <p>{{ viewState.detail }}</p>
      </div>
      <q-btn
        aria-label="Refresh game server metrics"
        color="primary"
        :disable="loading"
        :loading="loading"
        no-caps
        outline
        @click="fetchMetrics">
        <q-icon aria-hidden="true" class="q-mr-xs" name="refresh" />
        Refresh
      </q-btn>
    </header>

    <section aria-label="Metrics time range" class="metrics-toolbar">
      <q-btn-toggle
        v-model="selectedRange"
        aria-label="Select metrics time range"
        :options="metricsRangeOptions"
        dense
        no-caps
        spread
        toggle-color="primary" />
      <div class="metrics-toolbar__metadata">
        <span>{{ resolution }}</span>
        <span v-if="sampleIntervalSeconds > 0">{{ sampleIntervalSeconds }}s base interval</span>
        <span>{{ samples.length }} plotted points</span>
        <span v-if="latestSample">
          Latest
          {{ formatMetricAge(latestSample.processCollectedAtMs ?? latestSample.timestampMs) }}
        </span>
        <span v-if="currentRange.live" class="metrics-toolbar__live">
          <span aria-hidden="true" /> Live updates
        </span>
      </div>
    </section>

    <q-banner v-if="error !== ''" class="metrics-inline-state metrics-inline-state--error">
      <template #avatar><q-icon aria-hidden="true" name="error_outline" /></template>
      <strong>Metrics could not be loaded.</strong> {{ error }}
      <template #action>
        <q-btn color="negative" flat label="Retry" no-caps @click="fetchMetrics" />
      </template>
    </q-banner>

    <template v-if="loading && samples.length === 0">
      <section
        aria-label="Loading current metrics"
        class="metrics-current metrics-current--loading">
        <q-skeleton v-for="index in 5" :key="index" height="72px" type="rect" />
      </section>
      <div class="metrics-chart-grid">
        <q-skeleton v-for="index in 4" :key="index" height="310px" type="rect" />
      </div>
    </template>

    <section
      v-else-if="samples.length === 0 && error === ''"
      class="metrics-empty"
      aria-labelledby="metrics-empty-title">
      <q-icon aria-hidden="true" name="show_chart" size="40px" />
      <h2 id="metrics-empty-title">No telemetry in this range</h2>
      <p>
        Metrics are recorded while the server and its assigned node are reachable. Try a longer
        range or start the server to collect new samples.
      </p>
      <div>
        <q-btn color="primary" label="Try 24 hours" no-caps @click="selectedRange = '24h'" />
        <q-btn class="q-ml-sm" flat label="Retry" no-caps @click="fetchMetrics" />
      </div>
    </section>

    <template v-else-if="samples.length > 0">
      <section aria-label="Current server telemetry" class="metrics-current">
        <div class="metrics-current__item">
          <span>Process CPU</span>
          <strong class="font-mono">{{
            formatMetricPercent(latestSample?.cpuAverage ?? null)
          }}</strong>
          <small>{{ cpuCoreEquivalent }}</small>
        </div>
        <div class="metrics-current__item">
          <span>Process RSS</span>
          <strong class="font-mono">{{
            formatMetricBytes(currentCapacity.processRssBytes)
          }}</strong>
          <small>
            {{ formatMetricPercent(ratioToPercent(currentCapacity.configuredTargetRatio)) }} of
            configured target
          </small>
        </div>
        <div class="metrics-current__item">
          <span>Configured memory</span>
          <strong class="font-mono">{{
            formatMetricBytes(currentCapacity.configuredTargetBytes)
          }}</strong>
          <small>Server target, not node capacity</small>
        </div>
        <div class="metrics-current__item">
          <span>Node RAM available</span>
          <strong class="font-mono">
            {{ formatMetricBytes(currentCapacity.nodeAvailableBytes) }} /
            {{ formatMetricBytes(currentCapacity.nodeTotalBytes) }}
          </strong>
          <small>
            Process uses
            {{ formatMetricPercent(ratioToPercent(currentCapacity.nodeProcessShareRatio)) }}
            of node RAM
          </small>
        </div>
        <div class="metrics-current__item">
          <span>Server volume</span>
          <strong class="font-mono">{{ volumeCapacityLabel }}</strong>
          <small>{{ volumeFreshness }}</small>
        </div>
      </section>

      <section class="metrics-section" aria-labelledby="resource-metrics-title">
        <header class="metrics-section__header">
          <h2 id="resource-metrics-title">Resources</h2>
          <p>Process pressure against node CPU and configured memory limits.</p>
        </header>
        <div class="metrics-chart-grid">
          <metric-time-series-chart
            title="CPU utilization"
            description="Host-normalized process CPU. The peak line preserves short spikes in rollups."
            empty-label="CPU collection is not available for this range."
            :format-value="formatMetricPercent"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="cpuSeries"
            :summary="cpuSummary"
            :y-axis-maximum="100" />
          <metric-time-series-chart
            title="Process memory"
            description="Resident memory (RSS) compared with the configured server target."
            empty-label="Process memory was not recorded in this range."
            :format-value="formatMetricBytes"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="memorySeries"
            :summary="memorySummary" />
        </div>
      </section>

      <section class="metrics-section" aria-labelledby="storage-activity-title">
        <header class="metrics-section__header">
          <h2 id="storage-activity-title">Storage & activity</h2>
          <p>Directory growth, disk throughput, and active network connections.</p>
        </header>
        <div class="metrics-chart-grid">
          <metric-time-series-chart
            title="Server directory size"
            description="Measured server files on disk; volume capacity is shown separately above."
            empty-label="The directory size scan has not completed."
            :format-value="formatMetricBytes"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="storageSeries"
            :summary="storageSummary" />
          <metric-time-series-chart
            title="Disk I/O rate"
            description="Process read and write throughput per second."
            empty-label="Disk I/O rates are unavailable for this platform or range."
            :format-value="formatMetricRate"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="ioSeries"
            :summary="ioSummary" />
          <metric-time-series-chart
            title="Connections"
            description="Open process network connections; zero is distinct from an unavailable sample."
            empty-label="Connection counts are unavailable for this platform or range."
            :format-value="formatWholeNumber"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="connectionSeries"
            :summary="connectionSummary" />
        </div>
      </section>

      <section class="metrics-section" aria-labelledby="game-health-title">
        <header class="metrics-section__header">
          <h2 id="game-health-title">Game health</h2>
          <p>
            Capability-gated query telemetry. Unsupported values remain unknown rather than zero.
          </p>
        </header>
        <div class="metrics-chart-grid">
          <metric-time-series-chart
            title="Players"
            description="Reported players and server capacity from the game query protocol."
            :empty-label="playerEmptyLabel"
            :format-value="formatWholeNumber"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="playerSeries"
            :summary="playerSummary" />
          <metric-time-series-chart
            title="Query latency"
            description="Controller round-trip time for the game query. Gaps represent failed queries."
            :empty-label="queryEmptyLabel"
            :format-value="formatMilliseconds"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="querySeries"
            :summary="querySummary" />
          <metric-time-series-chart
            title="Server FPS"
            description="Authoritative server FPS when the game integration exposes it."
            empty-label="This game integration does not expose authoritative FPS telemetry."
            :format-value="formatFps"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="fpsSeries"
            :summary="fpsSummary" />
          <metric-time-series-chart
            title="Server frame time"
            description="Authoritative time spent producing each server frame; lower and steadier is better."
            empty-label="This game integration does not expose authoritative frame-time telemetry."
            :format-value="formatMilliseconds"
            :range-duration-ms="currentRange.durationMs"
            :samples="samples"
            :series="frameTimeSeries"
            :summary="frameTimeSummary" />
        </div>
      </section>

      <metrics-event-timeline :events="timeline" />
    </template>
  </main>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import MetricTimeSeriesChart, {
  type MetricChartSeries,
} from '@/components/game_servers/MetricTimeSeriesChart.vue'
import MetricsEventTimeline from '@/components/game_servers/MetricsEventTimeline.vue'
import {
  getMetricsRangeOption,
  hasCompleteVolumeCapacity,
  metricsRangeOptions,
  summarizeMetric,
  type MetricSample,
} from './game-server-metrics'
import {
  formatMetricAge,
  formatMetricBytes,
  formatMetricNumber,
  formatMetricPercent,
  formatMetricRate,
} from './metrics-format'
import { useGameServerMetrics } from './useGameServerMetrics'

const route = useRoute()
const gameServerId = computed(() => {
  const routeId = route.params.id
  return Array.isArray(routeId) ? (routeId[0] ?? '') : (routeId ?? '')
})

const {
  currentCapacity,
  error,
  fetchMetrics,
  latestSample,
  loading,
  resolution,
  sampleIntervalSeconds,
  samples,
  selectedRange,
  timeline,
  viewState,
} = useGameServerMetrics({ gameServerId })

const currentRange = computed(() => getMetricsRangeOption(selectedRange.value))

function summary(
  value: (sample: MetricSample) => number | null,
  minimum: (sample: MetricSample) => number | null,
  maximum: (sample: MetricSample) => number | null,
  validSampleCount: (sample: MetricSample) => number,
) {
  return computed(() =>
    summarizeMetric(samples.value, { value, minimum, maximum, validSampleCount }),
  )
}

const cpuSummary = summary(
  (sample) => sample.cpuAverage,
  (sample) => sample.cpuMinimum,
  (sample) => sample.cpuMaximum,
  (sample) => sample.cpuValidSampleCount,
)
const memorySummary = summary(
  (sample) => sample.memoryRssAverage,
  (sample) => sample.memoryRssMinimum,
  (sample) => sample.memoryRssMaximum,
  (sample) => sample.availableSampleCount,
)
const storageSummary = summary(
  (sample) => sample.diskUsageAverage,
  (sample) => sample.diskUsageMinimum,
  (sample) => sample.diskUsageMaximum,
  (sample) => sample.volumeValidSampleCount,
)
const ioSummary = summary(
  (sample) => sample.ioReadAverage,
  (sample) => sample.ioReadMinimum,
  (sample) => sample.ioReadMaximum,
  (sample) => sample.ioValidSampleCount,
)
const connectionSummary = summary(
  (sample) => sample.connectionAverage,
  (sample) => sample.connectionMinimum,
  (sample) => sample.connectionMaximum,
  (sample) => sample.connectionValidSampleCount,
)
const playerSummary = summary(
  (sample) => sample.playerAverage,
  (sample) => sample.playerMinimum,
  (sample) => sample.playerMaximum,
  (sample) => sample.querySuccessfulSampleCount,
)
const querySummary = summary(
  (sample) => sample.queryDurationAverage,
  (sample) => sample.queryDurationMinimum,
  (sample) => sample.queryDurationMaximum,
  (sample) => sample.queryDurationValidSampleCount,
)
const fpsSummary = summary(
  (sample) => sample.serverFpsAverage,
  (sample) => sample.serverFpsMinimum,
  (sample) => sample.serverFpsMaximum,
  (sample) => sample.serverFpsValidSampleCount,
)
const frameTimeSummary = summary(
  (sample) => sample.frameTimeAverage,
  (sample) => sample.frameTimeMinimum,
  (sample) => sample.frameTimeMaximum,
  (sample) => sample.serverFrameTimeValidSampleCount,
)

const cpuSeries: MetricChartSeries[] = [
  { label: 'Average', colorToken: '--xy-chart-1', value: (sample) => sample.cpuAverage },
  {
    label: 'Peak',
    colorToken: '--xy-chart-4',
    value: (sample) => sample.cpuMaximum,
    dashed: true,
  },
]
const memorySeries: MetricChartSeries[] = [
  { label: 'RSS', colorToken: '--xy-chart-2', value: (sample) => sample.memoryRssAverage },
  {
    label: 'Configured target',
    colorToken: '--xy-chart-4',
    value: (sample) => sample.configuredMemoryBytes,
    dashed: true,
  },
]
const storageSeries: MetricChartSeries[] = [
  {
    label: 'Directory size',
    colorToken: '--xy-chart-3',
    value: (sample) => sample.diskUsageAverage,
  },
]
const ioSeries: MetricChartSeries[] = [
  { label: 'Read', colorToken: '--xy-chart-1', value: (sample) => sample.ioReadAverage },
  { label: 'Write', colorToken: '--xy-chart-2', value: (sample) => sample.ioWriteAverage },
]
const connectionSeries: MetricChartSeries[] = [
  {
    label: 'Connections',
    colorToken: '--xy-chart-5',
    value: (sample) => sample.connectionAverage,
  },
]
const playerSeries: MetricChartSeries[] = [
  { label: 'Players', colorToken: '--xy-chart-2', value: (sample) => sample.playerAverage },
  {
    label: 'Capacity',
    colorToken: '--xy-chart-4',
    value: (sample) => sample.playerCapacity,
    dashed: true,
  },
]
const querySeries: MetricChartSeries[] = [
  {
    label: 'Latency',
    colorToken: '--xy-chart-1',
    value: (sample) => sample.queryDurationAverage,
  },
  {
    label: 'Peak',
    colorToken: '--xy-chart-3',
    value: (sample) => sample.queryDurationMaximum,
    dashed: true,
  },
]
const fpsSeries: MetricChartSeries[] = [
  { label: 'Server FPS', colorToken: '--xy-chart-2', value: (sample) => sample.serverFpsAverage },
  {
    label: 'Low',
    colorToken: '--xy-chart-4',
    value: (sample) => sample.serverFpsMinimum,
    dashed: true,
  },
]
const frameTimeSeries: MetricChartSeries[] = [
  {
    label: 'Average',
    colorToken: '--xy-chart-1',
    value: (sample) => sample.frameTimeAverage,
  },
  {
    label: 'Peak',
    colorToken: '--xy-chart-3',
    value: (sample) => sample.frameTimeMaximum,
    dashed: true,
  },
]

const cpuCoreEquivalent = computed(() => {
  const sample = latestSample.value
  if (!sample || sample.cpuAverage === null || sample.nodeCpuCores === null)
    return 'Core equivalent unknown'
  return `${((sample.cpuAverage / 100) * sample.nodeCpuCores).toFixed(2)} core equivalent`
})

const volumeCapacityLabel = computed(() => {
  const sample = latestSample.value
  if (!hasCompleteVolumeCapacity(sample)) return 'Unavailable'
  return `${formatMetricBytes(sample.volumeFreeBytes)} free / ${formatMetricBytes(sample.volumeTotalBytes)}`
})

const volumeFreshness = computed(() => {
  const sample = latestSample.value
  if (!hasCompleteVolumeCapacity(sample)) return 'Volume capacity unavailable'
  return `${formatMetricPercent(sample.volumePercent)} used · measured ${formatMetricAge(sample.diskMeasuredAtMs)}`
})

const playerEmptyLabel = computed(() =>
  latestSample.value?.querySupported === false
    ? 'This game does not expose player counts through a supported query protocol.'
    : 'No successful player query was recorded in this range.',
)

const queryEmptyLabel = computed(() =>
  latestSample.value?.querySupported === false
    ? 'This game does not have a supported query protocol.'
    : 'No successful query latency sample was recorded in this range.',
)

function ratioToPercent(ratio: number | null): number | null {
  return ratio === null ? null : ratio * 100
}

function formatWholeNumber(value: number | null): string {
  return value === null ? 'Unknown' : Math.round(value).toLocaleString()
}

function formatMilliseconds(value: number | null): string {
  return value === null ? 'Unknown' : `${formatMetricNumber(value, 1)} ms`
}

function formatFps(value: number | null): string {
  return value === null ? 'Unknown' : `${formatMetricNumber(value, 1)} FPS`
}
</script>

<style scoped>
.metrics-page {
  display: grid;
  gap: var(--xy-space-lg);
  max-width: 1500px;
  margin: 0 auto;
}

.metrics-page__header,
.metrics-page__title-row,
.metrics-toolbar,
.metrics-toolbar__metadata {
  display: flex;
  align-items: center;
}

.metrics-page__header {
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.metrics-page__title-row {
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.metrics-page__header h1 {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.metrics-page__header p,
.metrics-section__header p {
  max-width: 70ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
}

.metrics-state {
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-3);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.metrics-state--available {
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg);
}

.metrics-state--stale,
.metrics-state--warming-up,
.metrics-state--offline {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
}

.metrics-state--error,
.metrics-state--collector-error,
.metrics-state--node-unavailable {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
}

.metrics-toolbar {
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.metrics-toolbar :deep(.q-btn-group) {
  flex: 0 0 auto;
}

.metrics-toolbar__metadata {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metrics-toolbar__live {
  color: var(--xy-success-text-soft);
}

.metrics-toolbar__live > span {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-right: var(--xy-space-xs);
  background: var(--xy-success);
  border-radius: var(--xy-radius-pill);
}

.metrics-inline-state {
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-lg);
}

.metrics-inline-state--error {
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
}

.metrics-current {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  background: var(--xy-surface-1);
  border-top: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
}

.metrics-current--loading {
  gap: var(--xy-space-sm);
  background: transparent;
  border: 0;
}

.metrics-current__item {
  min-width: 0;
  padding: var(--xy-space-md);
}

.metrics-current__item + .metrics-current__item {
  border-left: 1px solid var(--xy-border);
}

.metrics-current__item span,
.metrics-current__item small {
  display: block;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metrics-current__item strong {
  display: block;
  margin: var(--xy-space-xs) 0;
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
}

.metrics-section {
  display: grid;
  gap: var(--xy-space-sm);
}

.metrics-section__header h2,
.metrics-empty h2 {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
}

.metrics-section__header p {
  font-size: var(--xy-font-size-sm);
}

.metrics-chart-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-md);
}

.metrics-empty {
  display: grid;
  min-height: 320px;
  place-items: center;
  align-content: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-muted);
  text-align: center;
}

.metrics-empty p {
  max-width: 58ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

@media (max-width: 1100px) {
  .metrics-current {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .metrics-current__item + .metrics-current__item {
    border-left: 0;
  }

  .metrics-current__item:nth-child(even) {
    border-left: 1px solid var(--xy-border);
  }

  .metrics-current__item:nth-child(n + 3) {
    border-top: 1px solid var(--xy-border);
  }
}

@media (max-width: 760px) {
  .metrics-page {
    padding: var(--xy-space-md);
  }

  .metrics-page__header,
  .metrics-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .metrics-toolbar__metadata {
    justify-content: flex-start;
  }

  .metrics-chart-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 520px) {
  .metrics-current {
    grid-template-columns: minmax(0, 1fr);
  }

  .metrics-current__item:nth-child(even) {
    border-left: 0;
  }

  .metrics-current__item + .metrics-current__item {
    border-top: 1px solid var(--xy-border);
  }
}
</style>
