<template>
  <div>
    <div class="row items-center q-mb-sm">
      <div class="text-subtitle2">{{ title }}</div>
      <q-space />
      <q-btn-toggle
        v-model="selectedRange"
        flat
        dense
        no-caps
        toggle-color="primary"
        :options="rangeOptions"
        class="text-caption" />
    </div>
    <Line :data="chartData" :options="chartOptions" style="max-height: 180px" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js'
import { Line } from 'vue-chartjs'

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

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
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
