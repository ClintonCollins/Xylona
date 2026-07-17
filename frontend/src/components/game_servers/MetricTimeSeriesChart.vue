<template>
  <section class="metric-chart" :aria-labelledby="titleId">
    <header class="metric-chart__header">
      <div>
        <h3 :id="titleId" class="metric-chart__title">{{ title }}</h3>
        <p class="metric-chart__description">{{ description }}</p>
      </div>
      <span class="metric-chart__latest font-mono">{{ formatValue(summary.latest) }}</span>
    </header>

    <div v-if="hasValues" :aria-describedby="summaryId" class="metric-chart__visual" role="img">
      <line-chart aria-hidden="true" :data="chartData" :options="chartOptions" />
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
  </section>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, useId } from 'vue'
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
} from 'chart.js'
import { Line as LineChart } from 'vue-chartjs'
import type { MetricSample, MetricSummary } from '@/pages/game_servers/game-server-metrics'

ChartJS.register(LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

export interface MetricChartSeries {
  label: string
  colorToken: string
  value: (sample: MetricSample) => number | null
  dashed?: boolean
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
  }>(),
  { yAxisMaximum: undefined },
)

const titleId = `metric-chart-title-${useId()}`
const summaryId = `metric-chart-summary-${useId()}`
const prefersReducedMotion = ref(false)
let reducedMotionQuery: MediaQueryList | null = null

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

const accessibleSummary = computed(
  () =>
    `${props.title}. Latest ${props.formatValue(props.summary.latest)}. ` +
    `Minimum ${props.formatValue(props.summary.minimum)}, average ${props.formatValue(props.summary.average)}, ` +
    `maximum ${props.formatValue(props.summary.maximum)}. Coverage ${coverageLabel.value}.`,
)

const chartData = computed<ChartData<'line', ({ x: number; y: number } | null)[]>>(() => ({
  datasets: props.series.map((series) => ({
    label: series.label,
    data: props.samples.map((sample) => {
      const value = series.value(sample)
      return value === null ? null : { x: sample.timestampMs, y: value }
    }),
    borderColor: cssToken(series.colorToken),
    backgroundColor: 'transparent',
    borderDash: series.dashed ? [5, 4] : undefined,
    borderWidth: series.dashed ? 1 : 2,
    pointRadius: 0,
    pointHitRadius: 8,
    spanGaps: false,
    tension: 0.2,
  })),
}))

const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: prefersReducedMotion.value ? false : { duration: 180 },
  normalized: true,
  parsing: false,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: {
      display: props.series.length > 1,
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
      ticks: {
        color: cssToken('--xy-text-secondary'),
        callback: (value) => props.formatValue(Number(value)),
      },
    },
  },
}))

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

@media (max-width: 520px) {
  .metric-chart__header {
    align-items: baseline;
  }

  .metric-chart__summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
