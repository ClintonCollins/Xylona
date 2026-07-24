<template>
  <section
    class="metric-chart"
    :class="{
      'metric-chart--lane': variant === 'lane',
      'metric-chart--lane-warn': variant === 'lane' && health?.level === 'warn',
      'metric-chart--lane-danger': variant === 'lane' && health?.level === 'danger',
    }"
    :aria-labelledby="titleId">
    <template v-if="variant === 'card'">
      <header class="metric-chart__header">
        <div>
          <h3 :id="titleId" class="metric-chart__title">{{ title }}</h3>
          <p class="metric-chart__description">{{ description }}</p>
        </div>
        <span class="metric-chart__latest font-mono">{{ formatValue(displayedValue) }}</span>
      </header>

      <div v-if="hasValues" :aria-describedby="summaryId" class="metric-chart__visual" role="img">
        <line-chart
          ref="chartRef"
          aria-hidden="true"
          :data="chartData"
          :options="chartOptions"
          :plugins="chartPlugins" />
      </div>
      <div v-else class="metric-chart__empty">
        <q-icon aria-hidden="true" name="data_usage" size="22px" />
        <span>{{ emptyLabel }}</span>
      </div>

      <p :id="summaryId" class="metric-chart__sr-only">{{ accessibleSummary }}</p>
      <details class="metric-chart__details">
        <summary>Data summary and coverage</summary>
        <dl class="metric-chart__summary">
          <div>
            <dt>Minimum</dt>
            <dd>{{ formatValue(summary.minimum) }}</dd>
          </div>
          <div>
            <dt>Average</dt>
            <dd>{{ formatValue(summary.average) }}</dd>
          </div>
          <div>
            <dt>Maximum</dt>
            <dd>{{ formatValue(summary.maximum) }}</dd>
          </div>
          <div>
            <dt>Coverage</dt>
            <dd>{{ coverageLabel }}</dd>
          </div>
        </dl>
      </details>
    </template>

    <template v-else>
      <div class="metric-lane__gutter">
        <div class="metric-lane__name">
          <h3 :id="titleId">{{ title }}</h3>
          <span
            v-if="health && health.level !== 'ok' && health.level !== 'unknown'"
            :class="`metric-lane__badge--${health.level}`"
            class="metric-lane__badge">
            {{ healthGlyph }} {{ health.label }}
          </span>
        </div>
        <div class="metric-lane__value font-mono">
          {{ formatValue(displayedValue) }}
          <small v-if="laneCaption">{{ laneCaption }}</small>
        </div>
        <div
          v-if="laneHeight >= 64"
          class="metric-lane__aggregate font-mono"
          :title="`min ${formatValue(summary.minimum)} · avg ${formatValue(summary.average)} · max ${formatValue(summary.maximum)}`">
          min {{ formatValue(summary.minimum) }} · max {{ formatValue(summary.maximum) }}
        </div>
      </div>
      <div class="metric-lane__plot" :style="{ height: `${laneHeight}px` }">
        <div v-if="hasValues" :aria-describedby="summaryId" class="metric-lane__visual" role="img">
          <line-chart
            ref="chartRef"
            aria-hidden="true"
            :data="chartData"
            :options="chartOptions"
            :plugins="chartPlugins" />
        </div>
        <div v-else class="metric-lane__empty">{{ emptyLabel }}</div>
        <p :id="summaryId" class="metric-chart__sr-only">{{ accessibleSummary }}</p>
      </div>
    </template>
  </section>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import {
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type Plugin,
} from 'chart.js'
import { Line as LineChart } from 'vue-chartjs'
import type {
  MetricHealth,
  MetricSample,
  MetricSummary,
} from '@/pages/game_servers/game-server-metrics'
import {
  hoveredMetricTimestampMs,
  nearestSampleTimestampMs,
} from '@/pages/game_servers/metrics-crosshair'

ChartJS.register(LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

export interface MetricChartSeries {
  label: string
  colorToken: string
  value: (sample: MetricSample) => number | null
  dashed?: boolean
}

export interface MetricChartEvent {
  timestampMs: number
  tone: 'neutral' | 'positive' | 'warning' | 'negative'
  title: string
}

export interface MetricChartBand {
  from: number
  to?: number
  colorToken?: string
}

const props = withDefaults(
  defineProps<{
    title: string
    description: string
    emptyLabel: string
    samples: MetricSample[]
    series: MetricChartSeries[]
    summary: MetricSummary
    formatValue: (value: number | null) => string
    rangeDurationMs: number
    yAxisMaximum?: number
    events?: MetricChartEvent[]
    bands?: MetricChartBand[]
    variant?: 'card' | 'lane'
    laneHeight?: number
    laneCaption?: string
    health?: MetricHealth
  }>(),
  {
    yAxisMaximum: undefined,
    events: () => [],
    bands: () => [],
    variant: 'card',
    laneHeight: 64,
    laneCaption: '',
    health: undefined,
  },
)

const titleId = `metric-chart-title-${useId()}`
const summaryId = `metric-chart-summary-${useId()}`
const chartRef = ref<{ chart?: ChartJS<'line'> } | null>(null)
const prefersReducedMotion = ref(false)
let reducedMotionQuery: MediaQueryList | null = null

const eventToneTokens: Record<MetricChartEvent['tone'], string> = {
  neutral: '--xy-text-muted',
  positive: '--xy-success',
  warning: '--xy-warning',
  negative: '--xy-danger',
}

function cssToken(token: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(token).trim()
}

function formatAxisTime(timestamp: number): string {
  const options: Intl.DateTimeFormatOptions =
    props.rangeDurationMs >= 7 * 24 * 60 * 60 * 1000
      ? { month: 'short', day: 'numeric' }
      : props.rangeDurationMs >= 24 * 60 * 60 * 1000
        ? { weekday: 'short', hour: 'numeric' }
        : { hour: 'numeric', minute: '2-digit' }
  return new Intl.DateTimeFormat(undefined, options).format(timestamp)
}

const hasValues = computed(() =>
  props.series.some((series) => props.samples.some((sample) => series.value(sample) !== null)),
)

const coverageLabel = computed(() => {
  if (props.summary.coverageRatio === null) return 'Unknown'
  return `${(props.summary.coverageRatio * 100).toFixed(0)}% · ${props.summary.sampleCount} samples`
})

const hoveredSample = computed<MetricSample | null>(() => {
  const hovered = hoveredMetricTimestampMs.value
  if (hovered === null) return null
  return props.samples.find((sample) => sample.timestampMs === hovered) ?? null
})

const displayedValue = computed<number | null>(() => {
  const sample = hoveredSample.value
  const primarySeries = props.series[0]
  if (sample && primarySeries) return primarySeries.value(sample)
  return props.summary.latest
})

const healthGlyph = computed(() => {
  if (props.health?.level === 'danger') return '●'
  if (props.health?.level === 'warn') return '▲'
  return ''
})

const accessibleSummary = computed(
  () =>
    `${props.title}. Latest ${props.formatValue(props.summary.latest)}. ` +
    `Minimum ${props.formatValue(props.summary.minimum)}, average ${props.formatValue(props.summary.average)}, ` +
    `maximum ${props.formatValue(props.summary.maximum)}. Coverage ${coverageLabel.value}.`,
)

function tokenToRgba(token: string, alpha: number): string {
  const color = cssToken(token)
  const match = /^#([0-9a-f]{6})$/i.exec(color)
  if (!match) return 'transparent'
  const value = Number.parseInt(match[1] ?? '', 16)
  return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${alpha})`
}

// With `parsing: false`, Chart.js requires gap points as { x, y: null } — a
// literal null entry crashes the scale's data-limit scan.
const chartData = computed<ChartData<'line', { x: number; y: number | null }[]>>(() => ({
  datasets: props.series.map((series, index) => {
    const filled = index === 0 && !series.dashed
    return {
      label: series.label,
      data: props.samples.map((sample) => ({
        x: sample.timestampMs,
        y: series.value(sample),
      })),
      borderColor: cssToken(series.colorToken),
      backgroundColor: filled ? tokenToRgba(series.colorToken, 0.07) : 'transparent',
      fill: filled ? 'origin' : false,
      borderDash: series.dashed ? [5, 4] : undefined,
      borderWidth: series.dashed ? 1 : 2,
      pointRadius: 0,
      pointHitRadius: 8,
      spanGaps: false,
      tension: 0.2,
    }
  }),
}))

function drawBands(chart: ChartJS<'line'>): void {
  if (props.bands.length === 0) return
  const { ctx, chartArea } = chart
  const yScale = chart.scales['y']
  if (!chartArea || !yScale) return
  ctx.save()
  for (const band of props.bands) {
    const top = Math.max(yScale.getPixelForValue(band.to ?? yScale.max), chartArea.top)
    const bottom = Math.min(yScale.getPixelForValue(band.from), chartArea.bottom)
    if (bottom <= top) continue
    ctx.fillStyle = cssToken(band.colorToken ?? '--xy-warning-bg-faint')
    ctx.fillRect(chartArea.left, top, chartArea.right - chartArea.left, bottom - top)
  }
  ctx.restore()
}

function drawEventMarkers(chart: ChartJS<'line'>): void {
  if (props.events.length === 0) return
  const { ctx, chartArea } = chart
  const xScale = chart.scales['x']
  if (!chartArea || !xScale) return
  ctx.save()
  for (const event of props.events) {
    if (event.timestampMs < xScale.min || event.timestampMs > xScale.max) continue
    const x = xScale.getPixelForValue(event.timestampMs)
    const color = cssToken(eventToneTokens[event.tone])
    ctx.strokeStyle = color
    ctx.globalAlpha = 0.55
    ctx.lineWidth = 1
    ctx.setLineDash([4, 4])
    ctx.beginPath()
    ctx.moveTo(x, chartArea.top)
    ctx.lineTo(x, chartArea.bottom)
    ctx.stroke()
    ctx.setLineDash([])
    ctx.globalAlpha = 1
    ctx.fillStyle = color
    ctx.beginPath()
    ctx.arc(x, chartArea.top + 3, 3, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.restore()
}

function drawCrosshair(chart: ChartJS<'line'>): void {
  const hovered = hoveredMetricTimestampMs.value
  if (hovered === null) return
  const { ctx, chartArea } = chart
  const xScale = chart.scales['x']
  const yScale = chart.scales['y']
  if (!chartArea || !xScale || !yScale) return
  if (hovered < xScale.min || hovered > xScale.max) return

  const x = xScale.getPixelForValue(hovered)
  ctx.save()
  ctx.strokeStyle = cssToken('--xy-text-primary')
  ctx.globalAlpha = 0.35
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(x, chartArea.top)
  ctx.lineTo(x, chartArea.bottom)
  ctx.stroke()
  ctx.globalAlpha = 1

  const sample = props.samples.find((candidate) => candidate.timestampMs === hovered)
  if (sample) {
    for (const series of props.series) {
      const value = series.value(sample)
      if (value === null) continue
      const y = yScale.getPixelForValue(value)
      if (y < chartArea.top || y > chartArea.bottom) continue
      ctx.fillStyle = cssToken(series.colorToken)
      ctx.strokeStyle = cssToken('--xy-surface-0')
      ctx.lineWidth = 1.5
      ctx.beginPath()
      ctx.arc(x, y, 3.5, 0, Math.PI * 2)
      ctx.fill()
      ctx.stroke()
    }
  }
  ctx.restore()
}

const overlayPlugin: Plugin<'line'> = {
  id: 'xyMetricsOverlay',
  beforeDatasetsDraw: (chart) => {
    drawBands(chart)
  },
  afterDatasetsDraw: (chart) => {
    drawEventMarkers(chart)
    drawCrosshair(chart)
  },
  afterEvent: (_chart, args) => {
    if (args.event.type === 'mouseout') hoveredMetricTimestampMs.value = null
  },
}

const chartPlugins = [overlayPlugin]

const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: prefersReducedMotion.value ? false : { duration: 180 },
  normalized: true,
  parsing: false,
  interaction: { mode: 'index', intersect: false },
  layout:
    props.variant === 'lane' ? { padding: { top: 4, bottom: 4, left: 0, right: 0 } } : undefined,
  onHover: (event, _elements, chart) => {
    if (event.x === null) return
    const xScale = chart.scales['x']
    if (!xScale) return
    const timestamp = xScale.getValueForPixel(event.x)
    hoveredMetricTimestampMs.value =
      timestamp === undefined ? null : nearestSampleTimestampMs(props.samples, timestamp)
  },
  plugins: {
    legend: {
      display: props.variant === 'card' && props.series.length > 1,
      position: 'bottom',
      labels: { color: cssToken('--xy-text-secondary'), usePointStyle: true, boxWidth: 8 },
    },
    tooltip: {
      backgroundColor: cssToken('--xy-chart-tooltip-bg'),
      borderColor: cssToken('--xy-chart-tooltip-border'),
      borderWidth: 1,
      titleColor: cssToken('--xy-chart-tooltip-text'),
      bodyColor: cssToken('--xy-chart-tooltip-text'),
      callbacks: {
        title: (items) => {
          const timestamp = items[0]?.parsed.x
          return timestamp === undefined
            ? ''
            : new Intl.DateTimeFormat(undefined, {
                month: 'short',
                day: 'numeric',
                hour: 'numeric',
                minute: '2-digit',
                second: '2-digit',
              }).format(timestamp)
        },
        label: (context) => `${context.dataset.label}: ${props.formatValue(context.parsed.y)}`,
      },
    },
  },
  scales: {
    x: {
      type: 'linear',
      display: props.variant === 'card',
      min: props.samples[0]?.timestampMs,
      max: props.samples[props.samples.length - 1]?.timestampMs,
      grid: { display: false },
      ticks: {
        color: cssToken('--xy-text-secondary'),
        maxTicksLimit: 6,
        maxRotation: 0,
        callback: (value) => formatAxisTime(Number(value)),
      },
    },
    y: {
      beginAtZero: true,
      max: props.yAxisMaximum,
      grid: { color: cssToken('--xy-chart-grid') },
      border: props.variant === 'lane' ? { display: false } : undefined,
      ticks:
        props.variant === 'lane'
          ? { display: false }
          : {
              color: cssToken('--xy-text-secondary'),
              callback: (value) => props.formatValue(Number(value)),
            },
    },
  },
}))

watch(hoveredMetricTimestampMs, () => {
  chartRef.value?.chart?.draw()
})

onMounted(() => {
  if (typeof window.matchMedia !== 'function') return
  reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  prefersReducedMotion.value = reducedMotionQuery.matches
  reducedMotionQuery.addEventListener('change', updateReducedMotion)
})

function updateReducedMotion(event: MediaQueryListEvent): void {
  prefersReducedMotion.value = event.matches
}

onBeforeUnmount(() => {
  reducedMotionQuery?.removeEventListener('change', updateReducedMotion)
})
</script>

<style scoped>
.metric-chart {
  min-width: 0;
  padding: var(--xy-space-md);
  background: var(--xy-surface-raised-soft);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.metric-chart__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
  margin-bottom: var(--xy-space-sm);
}

.metric-chart__title {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  line-height: var(--xy-line-height-tight);
}

.metric-chart__description {
  max-width: 58ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metric-chart__latest {
  flex: 0 0 auto;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-lg);
}

.metric-chart__visual {
  height: 210px;
}

.metric-chart__empty {
  display: flex;
  min-height: 210px;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-muted);
}

.metric-chart__details {
  margin-top: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.metric-chart__details summary {
  width: fit-content;
  cursor: pointer;
}

.metric-chart__details summary:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 3px;
}

.metric-chart__summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--xy-space-sm);
  margin: var(--xy-space-sm) 0 0;
}

.metric-chart__summary div {
  min-width: 0;
}

.metric-chart__summary dt {
  color: var(--xy-text-muted);
}

.metric-chart__summary dd {
  margin: var(--xy-space-2xs) 0 0;
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.metric-chart__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* ---- Lane variant: gutter + full-width plot inside a recorder frame ---- */
.metric-chart--lane {
  display: grid;
  grid-template-columns: var(--metric-lane-gutter, 224px) minmax(0, 1fr);
  padding: 0;
  background: transparent;
  border: 0;
  border-radius: 0;
}

.metric-chart--lane-warn {
  background: var(--xy-warning-bg-faint);
}

.metric-chart--lane-danger {
  background: var(--xy-danger-bg-faint);
}

.metric-lane__gutter {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--xy-space-2xs);
  min-width: 0;
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-right: 1px solid var(--xy-border);
}

.metric-lane__name {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.metric-lane__name h3 {
  margin: 0;
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  line-height: var(--xy-line-height-tight);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-lane__badge {
  flex: 0 0 auto;
  padding: 0 var(--xy-space-sm);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  white-space: nowrap;
}

.metric-lane__badge--warn {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
}

.metric-lane__badge--danger {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
}

.metric-lane__value {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-base);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-lane__value small {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 400;
}

.metric-lane__aggregate {
  overflow: hidden;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-lane__plot {
  position: relative;
  min-width: 0;
}

.metric-lane__visual {
  position: absolute;
  inset: 0;
}

.metric-lane__empty {
  display: flex;
  height: 100%;
  align-items: center;
  padding: 0 var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

@media (max-width: 520px) {
  .metric-chart__header {
    align-items: baseline;
  }

  .metric-chart__summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
