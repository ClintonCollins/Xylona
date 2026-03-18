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
    <Line :data="chartData" :options="chartOptions" style="max-height: 250px" />
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

const defaultColors = ['#1976D2', '#26A69A', '#FF6384', '#FF9F40', '#9966FF']

const chartData = computed(() => ({
  labels: props.labels,
  datasets: props.datasets.map((ds, i) => ({
    label: ds.label,
    data: ds.data,
    borderColor: ds.borderColor ?? defaultColors[i % defaultColors.length],
    backgroundColor: ds.backgroundColor ?? 'transparent',
    borderWidth: 2,
    pointRadius: 0,
    tension: 0.3,
    fill: false,
  })),
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
    },
    tooltip: {
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
      },
      grid: {
        display: false,
      },
    },
    y: {
      min: 0,
      max: props.yAxisMax,
      ticks: {
        callback: (value: string | number) => `${value}${props.yAxisSuffix ?? ''}`,
      },
    },
  },
}))
</script>
