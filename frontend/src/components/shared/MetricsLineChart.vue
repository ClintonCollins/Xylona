<template>
  <div>
    <div class="row items-center q-mb-sm">
      <div class="text-subtitle2">{{ title }}</div>
      <q-space />
      <q-btn-toggle
        v-model="selectedRange"
        :options="rangeOptions"
        class="text-caption"
        dense
        flat
        no-caps
        toggle-color="primary" />
    </div>
    <div
      :aria-describedby="chartSummaryId"
      :aria-label="`${title} metrics chart`"
      class="metrics-chart__visual"
      role="img">
      <line-chart
        aria-hidden="true"
        :data="chartData"
        :options="chartOptions"
        style="max-height: 180px" />
    </div>
    <p :id="chartSummaryId" class="metrics-chart__sr-only">{{ chartSummary }}</p>
  </div>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Title,
  Tooltip,
} from 'chart.js'
import { Line as LineChart } from 'vue-chartjs'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
)

function getCssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

export interface Dataset {
  label: string
  data: number[]
  borderColor?: string
  backgroundColor?: string
}

const props = defineProps<{
  title: string
  labels: string[]
  datasets: Dataset[]
  yAxisSuffix?: string
  yAxisMax?: number
}>()

const emit = defineEmits<{
  rangeChange: [range: string]
}>()

const rangeOptions = [
  { label: '1h', value: '1h' },
  { label: '6h', value: '6h' },
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
]

const selectedRange = ref('1h')
const chartSummaryId = `metrics-chart-summary-${useId()}`
const prefersReducedMotion = ref(false)

let reducedMotionQuery: MediaQueryList | null = null

function updateReducedMotionPreference(event?: MediaQueryListEvent): void {
  prefersReducedMotion.value = event?.matches ?? reducedMotionQuery?.matches ?? false
}

onMounted(() => {
  if (typeof window.matchMedia !== 'function') {
    return
  }

  reducedMotionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  updateReducedMotionPreference()
  reducedMotionQuery.addEventListener('change', updateReducedMotionPreference)
})

onBeforeUnmount(() => {
  reducedMotionQuery?.removeEventListener('change', updateReducedMotionPreference)
})

watch(selectedRange, (val) => {
  emit('rangeChange', val)
})

const defaultColors = [
  getCssVar('--xy-chart-1'),
  getCssVar('--xy-chart-2'),
  getCssVar('--xy-chart-3'),
  getCssVar('--xy-chart-4'),
  getCssVar('--xy-chart-5'),
]

function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

const chartData = computed(() => ({
  labels: props.labels,
  datasets: props.datasets.map((ds, i) => {
    const borderColor = ds.borderColor ?? defaultColors[i % defaultColors.length]
    return {
      label: ds.label,
      data: ds.data,
      borderColor,
      backgroundColor: ds.backgroundColor ?? hexToRgba(borderColor, 0.1),
      borderWidth: 2,
      pointRadius: 0,
      tension: 0.3,
      fill: true,
    }
  }),
}))

const chartSummary = computed(() => {
  const latestLabel = props.labels[props.labels.length - 1]
  const latestValues = props.datasets.map((dataset) => {
    const latestValue = dataset.data[dataset.data.length - 1]
    if (latestValue === undefined || !Number.isFinite(latestValue)) {
      return `${dataset.label}: no data`
    }
    return `${dataset.label}: ${latestValue.toFixed(1)}${props.yAxisSuffix ?? ''}`
  })

  if (latestValues.length === 0) {
    return `${props.title}. No metric data is available for the selected ${selectedRange.value} range.`
  }

  const sampleDescription = latestLabel ? `Latest sample ${latestLabel}.` : 'Latest sample.'
  return `${props.title}. Selected range ${selectedRange.value}. ${sampleDescription} ${latestValues.join(', ')}.`
})

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: prefersReducedMotion.value ? false : { duration: 1000 },
  interaction: {
    mode: 'index' as const,
    intersect: false,
  },
  plugins: {
    legend: {
      display: props.datasets.length > 1,
      position: 'bottom' as const,
      labels: {
        color: getCssVar('--xy-text-secondary'),
      },
    },
    tooltip: {
      backgroundColor: getCssVar('--xy-chart-tooltip-bg'),
      titleColor: getCssVar('--xy-chart-tooltip-text'),
      bodyColor: getCssVar('--xy-chart-tooltip-text'),
      borderColor: getCssVar('--xy-chart-tooltip-border'),
      borderWidth: 1,
      callbacks: {
        label: (context: { dataset: { label?: string }; parsed: { y: number | null } }) => {
          const label = context.dataset.label ?? ''
          const value = context.parsed.y
          if (value === null) return label
          return `${label}: ${value.toFixed(1)}${props.yAxisSuffix ?? ''}`
        },
      },
    },
  },
  scales: {
    x: {
      ticks: {
        maxTicksLimit: 8,
        maxRotation: 0,
        color: getCssVar('--xy-text-secondary'),
      },
      grid: {
        display: false,
      },
    },
    y: {
      min: 0,
      max: props.yAxisMax,
      ticks: {
        color: getCssVar('--xy-text-secondary'),
        callback: (value: string | number) => `${value}${props.yAxisSuffix ?? ''}`,
      },
      grid: {
        color: getCssVar('--xy-chart-grid'),
      },
    },
  },
}))
</script>

<style scoped>
.metrics-chart__sr-only {
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
</style>
