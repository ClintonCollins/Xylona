<template>
  <main class="metrics-page xy-page-content">
    <page-header icon="insights" title="Performance & Health">
      <div aria-live="polite" class="metrics-page__status">
        <span :class="`metrics-state--${viewState.kind}`" class="metrics-state">
          {{ viewState.label }}
        </span>
        <span>{{ viewState.detail }}</span>
      </div>
      <div v-if="hasProcessData" aria-label="Health attention summary" class="metrics-triage">
        <span class="metrics-triage__label">Attention</span>
        <button
          v-for="item in health.attention"
          :key="item.key"
          :class="`metrics-triage__chip--${item.level}`"
          class="metrics-triage__chip"
          type="button"
          @click="focusMetric(item.key)">
          <span aria-hidden="true">{{ item.level === 'danger' ? '●' : '▲' }}</span>
          {{ item.label }}
        </button>
        <span v-if="health.nominalCount > 0" class="metrics-triage__chip metrics-triage__chip--ok">
          <span aria-hidden="true">✓</span>
          {{ nominalChipLabel }}
        </span>
      </div>
      <template #actions>
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
      </template>
    </page-header>

    <section aria-label="Metrics display controls" class="metrics-toolbar">
      <div class="metrics-toolbar__controls">
        <q-btn-toggle
          v-model="selectedRange"
          aria-label="Select metrics time range"
          class="xy-segmented-toggle"
          :options="metricsRangeOptions"
          dense
          no-caps
          toggle-color="primary" />
        <q-btn-toggle
          v-model="viewMode"
          aria-label="Select metrics view mode"
          class="xy-segmented-toggle"
          :options="viewModeOptions"
          dense
          no-caps
          toggle-color="primary" />
      </div>
      <div class="metrics-toolbar__metadata">
        <span>{{ resolution }}</span>
        <span v-if="sampleIntervalSeconds > 0">{{ sampleIntervalSeconds }}s base interval</span>
        <span>{{ samples.length }} plotted points</span>
        <span v-if="latestSample">
          Latest
          {{
            formatMetricAge(latestSample.processCollectedAtMs ?? latestSample.timestampMs, nowMs)
          }}
        </span>
        <span v-if="currentRange.live" class="metrics-toolbar__live">
          <span aria-hidden="true" /> Live updates
        </span>
        <span class="metrics-toolbar__hint"><kbd>V</kbd> switch view</span>
      </div>
    </section>

    <q-banner
      v-if="error !== ''"
      class="metrics-inline-state metrics-inline-state--error"
      role="alert">
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
      v-else-if="!hasProcessData && error === ''"
      class="metrics-empty"
      aria-labelledby="metrics-empty-title">
      <q-icon
        aria-hidden="true"
        :name="offlineWholeRange ? 'power_off' : 'show_chart'"
        size="40px" />
      <h2 id="metrics-empty-title">
        {{ offlineWholeRange ? 'Server offline for this range' : 'No telemetry in this range' }}
      </h2>
      <p v-if="offlineWholeRange">
        This server was offline for the whole range, so there is no process telemetry to plot. Start
        the server or try a longer range to see earlier runs.
      </p>
      <p v-else>
        Metrics are recorded while the server and its assigned node are reachable. Try a longer
        range or start the server to collect new samples.
      </p>
      <div>
        <q-btn
          v-if="nextLongerRange"
          color="primary"
          :label="`Try ${nextLongerRange.label}`"
          no-caps
          @click="selectedRange = nextLongerRange.value" />
        <q-btn class="q-ml-sm" flat label="Retry" no-caps @click="fetchMetrics" />
      </div>
    </section>

    <template v-else-if="hasProcessData">
      <section aria-label="Current server health" class="metrics-current">
        <div class="metrics-current__item">
          <span>Process CPU</span>
          <strong class="font-mono">{{
            formatMetricPercent(latestSample?.cpuAverage ?? null)
          }}</strong>
          <small>{{ cpuCoreEquivalent }}</small>
          <span :class="`metrics-health--${health.cpu.level}`" class="metrics-health">
            <span aria-hidden="true">{{ healthGlyph(health.cpu.level) }}</span>
            {{ health.cpu.label }}
          </span>
        </div>
        <div :class="attentionCellClass(health.memory.level)" class="metrics-current__item">
          <span>Process memory</span>
          <strong class="font-mono">{{ memoryCellValue }}</strong>
          <small>{{ memoryCellDetail }}</small>
          <span :class="`metrics-health--${health.memory.level}`" class="metrics-health">
            <span aria-hidden="true">{{ healthGlyph(health.memory.level) }}</span>
            {{ health.memory.label }}
          </span>
        </div>
        <div class="metrics-current__item">
          <span>Node RAM available</span>
          <strong class="font-mono">{{
            formatMetricBytes(currentCapacity.nodeAvailableBytes)
          }}</strong>
          <small>
            of {{ formatMetricBytes(currentCapacity.nodeTotalBytes) }} · process uses
            {{ formatMetricPercent(ratioToPercent(currentCapacity.nodeProcessShareRatio)) }}
          </small>
        </div>
        <div
          id="metrics-cell-volume"
          :class="attentionCellClass(health.volume.level)"
          class="metrics-current__item">
          <span>Node volume</span>
          <strong class="font-mono">{{ volumeCapacityLabel }}</strong>
          <small>{{ volumeFreshness }}</small>
          <span :class="`metrics-health--${health.volume.level}`" class="metrics-health">
            <span aria-hidden="true">{{ healthGlyph(health.volume.level) }}</span>
            {{ health.volume.label }}
          </span>
        </div>
        <div class="metrics-current__item">
          <span>Game query</span>
          <strong class="font-mono">{{ queryCellValue }}</strong>
          <small>{{ queryCellDetail }}</small>
          <span :class="`metrics-health--${health.query.level}`" class="metrics-health">
            <span aria-hidden="true">{{ healthGlyph(health.query.level) }}</span>
            {{ health.query.label }}
          </span>
        </div>
      </section>

      <transition mode="out-in" name="metrics-view">
        <section
          v-if="viewMode === 'lanes'"
          key="lanes"
          aria-label="Flight recorder view"
          class="metrics-recorder">
          <div class="metrics-recorder__ruler">
            <span class="metrics-recorder__ruler-label">Flight recorder</span>
            <div class="metrics-recorder__ruler-track">
              <q-resize-observer @resize="rulerTrackWidth = $event.width" />
              <span
                v-for="tick in rulerTicks"
                :key="tick.percent"
                :class="tick.edge ? `metrics-recorder__anchor--${tick.edge}` : ''"
                class="metrics-recorder__tick font-mono"
                :style="{ left: `${tick.percent}%` }">
                {{ tick.label }}
              </span>
              <span
                v-for="event in rulerEvents"
                :key="event.key"
                :class="[
                  `metrics-recorder__event--${event.tone}`,
                  event.edge ? `metrics-recorder__anchor--${event.edge}` : '',
                ]"
                class="metrics-recorder__event font-mono"
                :style="{ left: `${event.percent}%` }"
                :title="event.detail">
                {{ event.label }}
              </span>
              <span
                v-if="rulerFlag"
                :class="rulerFlag.edge ? `metrics-recorder__anchor--${rulerFlag.edge}` : ''"
                class="metrics-recorder__flag font-mono"
                :style="{ left: `${rulerFlag.percent}%` }">
                {{ rulerFlag.label }}
              </span>
            </div>
          </div>
          <div class="metrics-recorder__lanes">
            <metric-time-series-chart
              id="metrics-panel-cpu"
              title="CPU"
              description="Host-normalized process CPU."
              empty-label="CPU collection is not available for this range."
              :bands="cpuBands"
              :events="chartEvents"
              :format-value="formatMetricPercent"
              :health="health.cpu"
              :lane-caption="cpuCoreEquivalent"
              :lane-height="96"
              :samples="samples"
              :series="cpuSeries"
              :summary="cpuSummary"
              variant="lane"
              :y-axis-maximum="100" />
            <metric-time-series-chart
              id="metrics-panel-memory"
              title="Memory"
              :description="memoryDescription"
              empty-label="Process memory was not recorded in this range."
              :bands="memoryBands"
              :events="chartEvents"
              :format-value="formatMetricBytes"
              :health="health.memory"
              :lane-caption="memoryLaneCaption"
              :lane-height="96"
              :samples="samples"
              :series="memorySeries"
              :summary="memorySummary"
              variant="lane" />
            <metric-time-series-chart
              v-if="querySupported"
              title="Players"
              description="Reported players and server capacity."
              :empty-label="playerEmptyLabel"
              :events="chartEvents"
              :format-value="formatWholeNumber"
              :lane-caption="playerLaneCaption"
              :lane-height="64"
              :samples="samples"
              :series="playerSeries"
              :summary="playerSummary"
              variant="lane" />
            <metric-time-series-chart
              v-if="querySupported"
              id="metrics-panel-query"
              title="Query latency"
              description="Controller round-trip time for the game query."
              :empty-label="queryEmptyLabel"
              :events="chartEvents"
              :format-value="formatMilliseconds"
              :health="health.query"
              lane-caption="gap = failed query"
              :lane-height="64"
              :samples="samples"
              :series="querySeries"
              :summary="querySummary"
              variant="lane" />
            <metric-time-series-chart
              title="Disk I/O"
              description="Process read plus write throughput per second."
              empty-label="Disk I/O rates are unavailable for this platform or range."
              :events="chartEvents"
              :format-value="formatMetricRate"
              :lane-caption="ioLaneCaption"
              :lane-height="64"
              :samples="samples"
              :series="ioSeries"
              :summary="ioSummary"
              variant="lane" />
            <metric-time-series-chart
              v-if="hasFpsData"
              title="Server FPS"
              description="Authoritative server FPS from the game integration."
              empty-label="No FPS telemetry in this range."
              :events="chartEvents"
              :format-value="formatFps"
              :lane-height="64"
              :samples="samples"
              :series="fpsSeries"
              :summary="fpsSummary"
              variant="lane" />
            <metric-time-series-chart
              v-if="hasFrameTimeData"
              title="Frame time"
              description="Time spent producing each server frame."
              empty-label="No frame-time telemetry in this range."
              :events="chartEvents"
              :format-value="formatMilliseconds"
              :lane-height="64"
              :samples="samples"
              :series="frameTimeSeries"
              :summary="frameTimeSummary"
              variant="lane" />
            <metric-time-series-chart
              title="Directory size"
              description="Measured server files on disk."
              empty-label="The directory size scan has not completed."
              :events="chartEvents"
              :format-value="formatMetricBytes"
              :lane-height="44"
              :samples="samples"
              :series="storageSeries"
              :summary="storageSummary"
              variant="lane" />
            <metric-time-series-chart
              title="Connections"
              description="Open process network connections."
              empty-label="Connection counts are unavailable for this platform or range."
              :events="chartEvents"
              :format-value="formatWholeNumber"
              :lane-height="44"
              :samples="samples"
              :series="connectionSeries"
              :summary="connectionSummary"
              variant="lane" />
          </div>
          <div v-if="unsupportedRows.length > 0" class="metrics-recorder__foot">
            <span v-for="row in unsupportedRows" :key="row">◌ {{ row }}</span>
          </div>
        </section>

        <div v-else key="grid" class="metrics-grid-view">
          <section class="metrics-section" aria-labelledby="resource-metrics-title">
            <header class="metrics-section__header">
              <h2 id="resource-metrics-title">Resources</h2>
              <p>Process pressure against node CPU and configured memory limits.</p>
            </header>
            <div class="metrics-chart-grid">
              <metric-time-series-chart
                id="metrics-panel-cpu"
                title="CPU utilization"
                description="Host-normalized process CPU. The peak line preserves short spikes in rollups."
                empty-label="CPU collection is not available for this range."
                :bands="cpuBands"
                :events="chartEvents"
                :format-value="formatMetricPercent"
                :samples="samples"
                :series="cpuSeries"
                :summary="cpuSummary"
                :y-axis-maximum="100" />
              <metric-time-series-chart
                id="metrics-panel-memory"
                title="Process memory"
                :description="memoryDescription"
                empty-label="Process memory was not recorded in this range."
                :bands="memoryBands"
                :events="chartEvents"
                :format-value="formatMetricBytes"
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
                :events="chartEvents"
                :format-value="formatMetricBytes"
                :samples="samples"
                :series="storageSeries"
                :summary="storageSummary" />
              <metric-time-series-chart
                title="Disk I/O rate"
                description="Process read and write throughput per second; the headline value is read plus write."
                empty-label="Disk I/O rates are unavailable for this platform or range."
                :events="chartEvents"
                :format-value="formatMetricRate"
                :samples="samples"
                :series="ioSeries"
                :summary="ioSummary" />
              <metric-time-series-chart
                title="Connections"
                description="Open process network connections; zero is distinct from an unavailable sample."
                empty-label="Connection counts are unavailable for this platform or range."
                :events="chartEvents"
                :format-value="formatWholeNumber"
                :samples="samples"
                :series="connectionSeries"
                :summary="connectionSummary" />
            </div>
          </section>

          <section class="metrics-section" aria-labelledby="game-health-title">
            <header class="metrics-section__header">
              <h2 id="game-health-title">Game health</h2>
              <p>
                Capability-gated query telemetry. Unsupported values remain unknown rather than
                zero.
              </p>
            </header>
            <div class="metrics-chart-grid">
              <metric-time-series-chart
                v-if="querySupported"
                title="Players"
                description="Reported players and server capacity from the game query protocol."
                :empty-label="playerEmptyLabel"
                :events="chartEvents"
                :format-value="formatWholeNumber"
                :samples="samples"
                :series="playerSeries"
                :summary="playerSummary" />
              <metric-time-series-chart
                v-if="querySupported"
                id="metrics-panel-query"
                title="Query latency"
                description="Controller round-trip time for the game query. Gaps represent failed queries."
                :empty-label="queryEmptyLabel"
                :events="chartEvents"
                :format-value="formatMilliseconds"
                :samples="samples"
                :series="querySeries"
                :summary="querySummary" />
              <metric-time-series-chart
                v-if="hasFpsData"
                title="Server FPS"
                description="Authoritative server FPS when the game integration exposes it."
                empty-label="No FPS telemetry in this range."
                :events="chartEvents"
                :format-value="formatFps"
                :samples="samples"
                :series="fpsSeries"
                :summary="fpsSummary" />
              <metric-time-series-chart
                v-if="hasFrameTimeData"
                title="Server frame time"
                description="Authoritative time spent producing each server frame; lower and steadier is better."
                empty-label="No frame-time telemetry in this range."
                :events="chartEvents"
                :format-value="formatMilliseconds"
                :samples="samples"
                :series="frameTimeSeries"
                :summary="frameTimeSummary" />
              <div
                v-if="unsupportedRows.length > 0"
                class="metrics-unsupported"
                :class="{ 'metrics-unsupported--full': !querySupported && !hasFpsData }">
                <span v-for="row in unsupportedRows" :key="row">◌ {{ row }}</span>
              </div>
            </div>
          </section>
        </div>
      </transition>
    </template>

    <metrics-event-timeline v-if="hasProcessData || timeline.length > 0" :events="timeline" />
  </main>
</template>

<script lang="ts" setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type {
  MetricChartBand,
  MetricChartEvent,
  MetricChartSeries,
} from '@/components/game_servers/MetricTimeSeriesChart.vue'
import MetricsEventTimeline from '@/components/game_servers/MetricsEventTimeline.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import { formatTime } from '@/utils/format-timestamp'
import {
  clusterRulerLabels,
  deriveServerHealth,
  getMetricsRangeOption,
  heapDangerRatio,
  heapWarnRatio,
  hasCompleteVolumeCapacity,
  isMetricsRangeKey,
  metricsCollectionStatus,
  metricsRangeOptions,
  rulerAnchor,
  summarizeMetric,
  type MetricHealthLevel,
  type MetricSample,
  type ServerHealthAttentionItem,
} from './game-server-metrics'
import { hoveredMetricTimestampMs } from './metrics-crosshair'
import {
  formatMetricAge,
  formatMetricAxisTime,
  formatMetricBytes,
  formatMetricNumber,
  formatMetricPercent,
  formatMetricRate,
  formatMetricTimestamp,
} from './metrics-format'
import { useGameServerMetrics, type MetricsTimelineEvent } from './useGameServerMetrics'

const MetricTimeSeriesChart = defineAsyncComponent(
  () => import('@/components/game_servers/MetricTimeSeriesChart.vue'),
)

type MetricsViewMode = 'lanes' | 'grid'

const viewModeStorageKey = 'xy-metrics-view'
const viewModeOptions = [
  { label: 'Lanes', value: 'lanes', icon: 'table_rows' },
  { label: 'Grid', value: 'grid', icon: 'grid_view' },
]

const route = useRoute()
const router = useRouter()
const gameServerId = computed(() => {
  const routeId = route.params.id
  return Array.isArray(routeId) ? (routeId[0] ?? '') : (routeId ?? '')
})

function queryValue(value: unknown): string {
  return Array.isArray(value) ? String(value[0] ?? '') : String(value ?? '')
}

function isMetricsViewMode(value: unknown): value is MetricsViewMode {
  return value === 'lanes' || value === 'grid'
}

function resolveInitialViewMode(): MetricsViewMode {
  const fromRoute = queryValue(route.query.view)
  if (isMetricsViewMode(fromRoute)) return fromRoute
  try {
    const stored = window.localStorage.getItem(viewModeStorageKey)
    if (isMetricsViewMode(stored)) return stored
  } catch {
    // localStorage can be unavailable (private mode, embedded webviews); default below.
  }
  return 'lanes'
}

function resolveInitialRange() {
  const fromRoute = queryValue(route.query.range)
  return isMetricsRangeKey(fromRoute) ? fromRoute : undefined
}

const viewMode = ref<MetricsViewMode>(resolveInitialViewMode())

const {
  currentCapacity,
  error,
  fetchMetrics,
  latestSample,
  loading,
  queryHealthSample,
  resolution,
  sampleIntervalSeconds,
  samples,
  selectedRange,
  timeline,
  viewState,
} = useGameServerMetrics({ gameServerId, initialRange: resolveInitialRange() })

watch([selectedRange, viewMode], ([range, view]) => {
  try {
    window.localStorage.setItem(viewModeStorageKey, view)
  } catch {
    // Persisting the preference is best-effort.
  }
  void router.replace({ query: { ...route.query, range, view } })
})

const nowMs = ref(Date.now())
let nowTimer: ReturnType<typeof setInterval> | undefined

function onViewShortcut(event: KeyboardEvent): void {
  if (event.key.toLowerCase() !== 'v' || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  if (
    target &&
    (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)
  )
    return
  viewMode.value = viewMode.value === 'lanes' ? 'grid' : 'lanes'
}

onMounted(() => {
  nowTimer = setInterval(() => {
    nowMs.value = Date.now()
  }, 1000)
  window.addEventListener('keydown', onViewShortcut)
})

onBeforeUnmount(() => {
  if (nowTimer !== undefined) clearInterval(nowTimer)
  window.removeEventListener('keydown', onViewShortcut)
  hoveredMetricTimestampMs.value = null
})

const currentRange = computed(() => getMetricsRangeOption(selectedRange.value))

const nextLongerRange = computed(() => {
  const index = metricsRangeOptions.findIndex((option) => option.value === selectedRange.value)
  return metricsRangeOptions[index + 1] ?? null
})

const health = computed(() =>
  deriveServerHealth({
    latestSample: latestSample.value,
    querySample: queryHealthSample.value,
    capacity: currentCapacity.value,
  }),
)

// Offline samples still count as samples, so "has data" means some process telemetry.
const hasProcessData = computed(() =>
  samples.value.some((sample) => sample.availableSampleCount > 0),
)
const offlineWholeRange = computed(
  () =>
    samples.value.length > 0 &&
    samples.value.every(
      (sample) => sample.collectionStatus === metricsCollectionStatus.serverOffline,
    ),
)

// "All" only when every health cell is nominal; unknown cells are not nominal.
const nominalChipLabel = computed(() => {
  const { attention, nominalCount, cpu, memory, volume, query } = health.value
  const total = [cpu, memory, volume, query].length
  return attention.length === 0 && nominalCount === total
    ? 'All metrics nominal'
    : `${nominalCount} nominal`
})

function healthGlyph(level: MetricHealthLevel): string {
  if (level === 'danger') return '●'
  if (level === 'warn') return '▲'
  if (level === 'ok') return '✓'
  return '◌'
}

function attentionCellClass(level: MetricHealthLevel): string {
  if (level === 'danger') return 'metrics-current__item--danger'
  if (level === 'warn') return 'metrics-current__item--warn'
  return ''
}

function focusMetric(key: ServerHealthAttentionItem['key']): void {
  const panelIds: Record<ServerHealthAttentionItem['key'], string> = {
    cpu: 'metrics-panel-cpu',
    memory: 'metrics-panel-memory',
    volume: 'metrics-cell-volume',
    query: 'metrics-panel-query',
  }
  const element = document.getElementById(panelIds[key])
  if (!element) return
  const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  element.scrollIntoView({ behavior: reduceMotion ? 'auto' : 'smooth', block: 'center' })
  element.animate?.(
    [
      { outline: '2px solid var(--xy-focus-ring)', outlineOffset: '2px' },
      { outline: '2px solid transparent', outlineOffset: '2px' },
    ],
    { duration: reduceMotion ? 0 : 1200, easing: 'ease-out' },
  )
}

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
function sumNullable(left: number | null, right: number | null): number | null {
  return left === null && right === null ? null : (left ?? 0) + (right ?? 0)
}

// Read plus write, so a write-heavy world save or backup shows up in the headline value.
const ioSummary = summary(
  (sample) => sumNullable(sample.ioReadAverage, sample.ioWriteAverage),
  (sample) => sumNullable(sample.ioReadMinimum, sample.ioWriteMinimum),
  (sample) => sumNullable(sample.ioReadMaximum, sample.ioWriteMaximum),
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
  { label: 'Average', colorToken: '--xy-series-1', value: (sample) => sample.cpuAverage },
  {
    label: 'Peak',
    colorToken: '--xy-series-neutral',
    value: (sample) => sample.cpuMaximum,
    dashed: true,
  },
]
const memorySeries: MetricChartSeries[] = [
  { label: 'RSS', colorToken: '--xy-series-2', value: (sample) => sample.memoryRssAverage },
  {
    label: 'Java heap limit',
    colorToken: '--xy-series-limit',
    value: (sample) => sample.configuredMemoryBytes,
    dashed: true,
  },
]
const storageSeries: MetricChartSeries[] = [
  {
    label: 'Directory size',
    colorToken: '--xy-series-3',
    value: (sample) => sample.diskUsageAverage,
  },
]
const ioSeries: MetricChartSeries[] = [
  {
    label: 'Read + write',
    colorToken: '--xy-series-neutral',
    value: (sample) => sumNullable(sample.ioReadAverage, sample.ioWriteAverage),
    dashed: true,
  },
  { label: 'Read', colorToken: '--xy-series-1', value: (sample) => sample.ioReadAverage },
  { label: 'Write', colorToken: '--xy-series-2', value: (sample) => sample.ioWriteAverage },
]
const connectionSeries: MetricChartSeries[] = [
  {
    label: 'Connections',
    colorToken: '--xy-series-3',
    value: (sample) => sample.connectionAverage,
  },
]
const playerSeries: MetricChartSeries[] = [
  { label: 'Players', colorToken: '--xy-series-2', value: (sample) => sample.playerAverage },
  {
    label: 'Capacity',
    colorToken: '--xy-series-limit',
    value: (sample) => sample.playerCapacity,
    dashed: true,
  },
]
const querySeries: MetricChartSeries[] = [
  {
    label: 'Latency',
    colorToken: '--xy-series-1',
    value: (sample) => sample.queryDurationAverage,
  },
  {
    label: 'Peak',
    colorToken: '--xy-series-neutral',
    value: (sample) => sample.queryDurationMaximum,
    dashed: true,
  },
]
const fpsSeries: MetricChartSeries[] = [
  { label: 'Server FPS', colorToken: '--xy-series-2', value: (sample) => sample.serverFpsAverage },
  {
    label: 'Low',
    colorToken: '--xy-series-neutral',
    value: (sample) => sample.serverFpsMinimum,
    dashed: true,
  },
]
const frameTimeSeries: MetricChartSeries[] = [
  {
    label: 'Average',
    colorToken: '--xy-series-1',
    value: (sample) => sample.frameTimeAverage,
  },
  {
    label: 'Peak',
    colorToken: '--xy-series-neutral',
    value: (sample) => sample.frameTimeMaximum,
    dashed: true,
  },
]

const cpuBands: MetricChartBand[] = [{ from: 85, to: 100 }]

const memoryBands = computed<MetricChartBand[]>(() => {
  const heap = currentCapacity.value.configuredTargetBytes
  if (heap === null || heap <= 0) return []
  return [
    { from: heap * heapWarnRatio, to: heap * heapDangerRatio },
    { from: heap * heapDangerRatio, colorToken: '--xy-danger-bg-faint' },
  ]
})

const chartEvents = computed<MetricChartEvent[]>(() => {
  const first = samples.value[0]
  const last = samples.value[samples.value.length - 1]
  if (!first || !last) return []
  return timeline.value
    .filter(
      (event) => event.timestampMs >= first.timestampMs && event.timestampMs <= last.timestampMs,
    )
    .map((event) => ({
      timestampMs: event.timestampMs,
      tone: event.tone,
      title: event.title,
    }))
})

const sampleTimeSpan = computed(() => {
  const first = samples.value[0]
  const last = samples.value[samples.value.length - 1]
  if (!first || !last || last.timestampMs <= first.timestampMs) return null
  return { minMs: first.timestampMs, maxMs: last.timestampMs }
})

const rulerTrackWidth = ref(0)
// Event labels are capped at this width in CSS; ticks need roughly the same room.
const rulerLabelWidthPx = 152

// Elements near the track edges anchor inward so they never clip outside it.
function rulerEdge(percent: number, halfWidthPx: number): 'start' | 'end' | null {
  const width = rulerTrackWidth.value
  if (width <= 0) return percent <= 8 ? 'start' : percent >= 92 ? 'end' : null
  return rulerAnchor((percent / 100) * width, width, halfWidthPx)
}

const rulerTicks = computed(() => {
  const span = sampleTimeSpan.value
  if (!span) return []
  const width = rulerTrackWidth.value
  const fractions =
    width === 0 || width >= 480 ? [0, 0.25, 0.5, 0.75, 1] : width >= 260 ? [0, 0.5, 1] : [0, 1]
  return fractions.map((fraction) => ({
    percent: fraction * 100,
    edge: rulerEdge(fraction * 100, 40),
    label: formatMetricAxisTime(
      span.minMs + fraction * (span.maxMs - span.minMs),
      span.maxMs - span.minMs,
    ),
  }))
})

const toneRank: Record<MetricsTimelineEvent['tone'], number> = {
  neutral: 0,
  positive: 1,
  warning: 2,
  negative: 3,
}

// Events whose labels would overlap collapse into one "N events" marker whose
// tooltip lists them, so neighbouring labels never draw over each other.
const rulerEvents = computed(() => {
  const span = sampleTimeSpan.value
  if (!span) return []
  // Until the track is measured, assume a laptop-width track.
  const width = rulerTrackWidth.value || 1000
  const positioned = [...chartEvents.value]
    .sort((a, b) => a.timestampMs - b.timestampMs)
    .map((event) => ({
      event,
      x: ((event.timestampMs - span.minMs) / (span.maxMs - span.minMs)) * width,
    }))
  return clusterRulerLabels(positioned, width, rulerLabelWidthPx, 8).map((cluster) => {
    const events = cluster.items.map((item) => item.event)
    const tone = events.reduce<MetricChartEvent['tone']>(
      (worst, event) => (toneRank[event.tone] > toneRank[worst] ? event.tone : worst),
      'neutral',
    )
    const percent = (cluster.x / width) * 100
    return {
      key: `${percent}-${events.length}`,
      percent,
      edge: cluster.anchor,
      tone,
      label: events.length === 1 ? (events[0]?.title ?? '') : `${events.length} events`,
      detail: events
        .map((event) => `${formatMetricTimestamp(event.timestampMs)} · ${event.title}`)
        .join('\n'),
    }
  })
})

const rulerFlag = computed(() => {
  const span = sampleTimeSpan.value
  const hovered = hoveredMetricTimestampMs.value
  if (!span || hovered === null || hovered < span.minMs || hovered > span.maxMs) return null
  const percent = ((hovered - span.minMs) / (span.maxMs - span.minMs)) * 100
  return {
    percent,
    edge: rulerEdge(percent, 40),
    label: formatTime(new Date(hovered)),
  }
})

const cpuCoreEquivalent = computed(() => {
  const sample = latestSample.value
  if (!sample || sample.cpuAverage === null || sample.nodeCpuCores === null)
    return 'Core equivalent unknown'
  return `${((sample.cpuAverage / 100) * sample.nodeCpuCores).toFixed(2)} core equivalent`
})

const memoryDescription = computed(() =>
  (currentCapacity.value.configuredTargetBytes ?? 0) > 0
    ? "Resident memory (RSS) against the Java heap limit; a JVM's RSS normally sits above its heap."
    : 'Resident memory (RSS) of the server process.',
)

// The value is the resident memory alone; the heap limit is context, not a ceiling.
const memoryCellValue = computed(() => formatMetricBytes(currentCapacity.value.processRssBytes))

const memoryCellDetail = computed(() => {
  const heap = currentCapacity.value.configuredTargetBytes
  const ratio = currentCapacity.value.configuredTargetRatio
  if (heap === null || heap <= 0) return 'No memory limit configured'
  const share = ratio === null ? '' : ` · ${formatMetricPercent(ratio * 100, 0)}`
  return `Java heap limit ${formatMetricBytes(heap)}${share}`
})

const memoryLaneCaption = computed(() => {
  const heap = currentCapacity.value.configuredTargetBytes
  if (heap === null || heap <= 0) return 'no limit set'
  return `heap limit ${formatMetricBytes(heap)}`
})

const ioLaneCaption = computed(() => {
  const sample = [...samples.value]
    .reverse()
    .find((candidate) => candidate.ioReadAverage !== null || candidate.ioWriteAverage !== null)
  if (!sample) return 'read + write'
  return `R ${formatMetricRate(sample.ioReadAverage)} · W ${formatMetricRate(sample.ioWriteAverage)}`
})

const playerLaneCaption = computed(() => {
  const capacity = queryHealthSample.value?.playerCapacity
  return capacity === null || capacity === undefined ? '' : `capacity ${capacity}`
})

const queryCellValue = computed(() => {
  const sample = queryHealthSample.value
  if (sample?.querySupported === false) return 'Not supported'
  return formatMilliseconds(sample?.queryDurationAverage ?? null)
})

const queryCellDetail = computed(() => {
  const sample = queryHealthSample.value
  if (sample?.querySupported === false) return 'No supported query protocol'
  if (!sample || sample.playerAverage === null) return 'Latency · players unknown'
  const capacity = sample.playerCapacity === null ? '' : `/${sample.playerCapacity}`
  return `Latency · ${Math.round(sample.playerAverage)}${capacity} players online`
})

const volumeCapacityLabel = computed(() => {
  const sample = latestSample.value
  if (!hasCompleteVolumeCapacity(sample)) return 'Unavailable'
  return `${formatMetricBytes(sample.volumeFreeBytes)} free`
})

const volumeFreshness = computed(() => {
  const sample = latestSample.value
  if (!hasCompleteVolumeCapacity(sample)) return 'Volume capacity unavailable'
  return `of ${formatMetricBytes(sample.volumeTotalBytes)} · ${formatMetricPercent(sample.volumePercent, 0)} used · measured ${formatMetricAge(sample.diskMeasuredAtMs, nowMs.value)}`
})

const querySupported = computed(() => queryHealthSample.value?.querySupported !== false)

const hasFpsData = computed(() => samples.value.some((sample) => sample.serverFpsAverage !== null))

const hasFrameTimeData = computed(() =>
  samples.value.some((sample) => sample.frameTimeAverage !== null),
)

const unsupportedRows = computed(() => {
  const rows: string[] = []
  if (!querySupported.value)
    rows.push('Players & query latency — no supported query protocol for this game')
  if (!hasFpsData.value) rows.push('Server FPS — not exposed by this game integration')
  if (!hasFrameTimeData.value) rows.push('Frame time — not exposed by this game integration')
  return rows
})

const playerEmptyLabel = computed(() =>
  queryHealthSample.value?.querySupported === false
    ? 'This game does not expose player counts through a supported query protocol.'
    : 'No successful player query was recorded in this range.',
)

const queryEmptyLabel = computed(() =>
  queryHealthSample.value?.querySupported === false
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
  gap: var(--xy-space-md);
}

.metrics-page > .xy-page-header {
  margin-bottom: 0;
}

.metrics-page__status,
.metrics-toolbar,
.metrics-toolbar__controls,
.metrics-toolbar__metadata {
  display: flex;
  align-items: center;
}

.metrics-page__status {
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
}

.metrics-triage {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-sm);
}

.metrics-triage__label {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.metrics-triage__chip {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-2xs) var(--xy-space-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-pill);
  background: transparent;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  cursor: pointer;
}

.metrics-triage__chip:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.metrics-triage__chip--warn {
  color: var(--xy-warning-hover);
  border-color: var(--xy-warning-border);
  background: var(--xy-warning-bg-faint);
}

.metrics-triage__chip--danger {
  color: var(--xy-danger-hover);
  border-color: var(--xy-danger-border);
  background: var(--xy-danger-bg-faint);
}

.metrics-triage__chip--ok {
  color: var(--xy-success-text-soft);
  border-color: var(--xy-success-border-softer);
  cursor: default;
}

.metrics-section__header p {
  max-width: 70ch;
  margin: var(--xy-space-2xs) 0 0;
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
  flex-wrap: wrap;
}

.metrics-toolbar__controls {
  gap: var(--xy-space-md);
  flex-wrap: wrap;
}

.metrics-toolbar__metadata {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metrics-toolbar__hint kbd {
  padding: 0 var(--xy-space-xs);
  border: 1px solid var(--xy-border-hover);
  border-bottom-width: 2px;
  border-radius: var(--xy-radius-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
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
  animation: metrics-live-pulse 2s var(--xy-ease-standard) infinite;
}

@keyframes metrics-live-pulse {
  0% {
    box-shadow: 0 0 0 0 var(--xy-success-border);
  }

  70% {
    box-shadow: 0 0 0 6px transparent;
  }

  100% {
    box-shadow: 0 0 0 0 transparent;
  }
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

.metrics-current__item--warn {
  background: var(--xy-warning-bg-faint);
}

.metrics-current__item--danger {
  background: var(--xy-danger-bg-faint);
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
  overflow: hidden;
  color: var(--xy-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
}

.metrics-current__item .metrics-health {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  margin-top: var(--xy-space-sm);
  padding: 0 var(--xy-space-sm);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
}

.metrics-current__item .metrics-health span {
  display: inline;
  color: inherit;
  font-size: inherit;
}

.metrics-current__item .metrics-health--ok {
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg);
}

.metrics-current__item .metrics-health--warn {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
}

.metrics-current__item .metrics-health--danger {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
}

.metrics-current__item .metrics-health--unknown {
  color: var(--xy-text-muted);
  background: var(--xy-surface-3);
}

.metrics-section {
  display: grid;
  gap: var(--xy-space-xs);
}

.metrics-grid-view {
  display: grid;
  gap: var(--xy-space-md);
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
  grid-template-columns: repeat(auto-fit, minmax(min(440px, 100%), 1fr));
  gap: var(--xy-space-md);
}

.metrics-unsupported {
  display: grid;
  gap: var(--xy-space-sm);
  align-content: start;
}

.metrics-unsupported--full {
  grid-column: 1 / -1;
}

.metrics-unsupported span {
  padding: var(--xy-space-sm) var(--xy-space-md);
  border: 1px dashed var(--xy-border);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-0);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}

/* ---- Flight recorder (lanes) view ---- */
.metrics-recorder {
  --metric-lane-gutter: 224px;

  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  overflow: hidden;
}

.metrics-recorder__ruler {
  display: grid;
  grid-template-columns: var(--metric-lane-gutter) minmax(0, 1fr);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.metrics-recorder__ruler-label {
  align-self: end;
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.metrics-recorder__ruler-track {
  position: relative;
  height: 44px;
  border-left: 1px solid var(--xy-border);
}

.metrics-recorder__tick {
  position: absolute;
  bottom: var(--xy-space-2xs);
  transform: translateX(-50%);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  white-space: nowrap;
}

.metrics-recorder__event {
  position: absolute;
  top: var(--xy-space-xs);
  max-width: 152px;
  overflow: hidden;
  padding: 0 var(--xy-space-sm);
  transform: translateX(-50%);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  background: var(--xy-surface-2);
  font-size: var(--xy-font-size-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metrics-recorder__event--positive {
  color: var(--xy-success-text-soft);
  border-color: var(--xy-success-border);
}

.metrics-recorder__event--warning {
  color: var(--xy-warning-hover);
  border-color: var(--xy-warning-border);
}

.metrics-recorder__event--negative {
  color: var(--xy-danger-hover);
  border-color: var(--xy-danger-border);
}

.metrics-recorder__event--neutral {
  color: var(--xy-text-secondary);
}

.metrics-recorder__flag {
  position: absolute;
  top: var(--xy-space-xs);
  z-index: 1;
  padding: 0 var(--xy-space-sm);
  transform: translateX(-50%);
  border-radius: var(--xy-radius-sm);
  background: var(--xy-text-primary);
  color: var(--xy-base);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  pointer-events: none;
  white-space: nowrap;
}

/* After the tick, event and flag rules so the edge anchoring overrides their centring. */
.metrics-recorder__anchor--start {
  transform: translateX(var(--xy-space-xs));
}

.metrics-recorder__anchor--end {
  transform: translateX(calc(-100% - var(--xy-space-xs)));
}

.metrics-recorder__lanes > .metric-chart--lane + .metric-chart--lane {
  border-top: 1px solid var(--xy-border);
}

.metrics-recorder__foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm) var(--xy-space-lg);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-top: 1px solid var(--xy-border);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

/* ---- View transition ---- */
.metrics-view-enter-active,
.metrics-view-leave-active {
  transition:
    opacity var(--xy-transition-fast),
    transform var(--xy-transition-fast);
}

.metrics-view-enter-from,
.metrics-view-leave-to {
  opacity: 0;
  transform: translateY(6px);
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

@media (max-width: 1023px) {
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

@media (max-width: 1023px) {
  .metrics-recorder {
    --metric-lane-gutter: 148px;
  }
}

@media (max-width: 599px) {
  .metrics-page {
    padding: var(--xy-space-md);
  }

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

@media (max-width: 599px) {
  .metrics-current {
    grid-template-columns: minmax(0, 1fr);
  }

  /* Lanes stack their label above the plot on phones, so the ruler spans the plot width. */
  .metrics-recorder__ruler {
    grid-template-columns: minmax(0, 1fr);
  }

  .metrics-recorder__ruler-label {
    display: none;
  }

  .metrics-recorder__ruler-track {
    border-left: 0;
  }

  .metrics-current__item:nth-child(even) {
    border-left: 0;
  }

  .metrics-current__item + .metrics-current__item {
    border-top: 1px solid var(--xy-border);
  }
}
</style>
