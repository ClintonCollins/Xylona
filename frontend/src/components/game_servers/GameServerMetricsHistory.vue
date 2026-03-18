<template>
  <div>
    <div v-if="!loading && historyPoints.length === 0" class="text-grey text-center q-pa-lg">
      No metrics data available for the selected time range. Metrics are recorded every 60 seconds while the server is running.
    </div>
    <template v-if="historyPoints.length > 0">
      <div class="q-mb-md">
        <MetricsLineChart
          title="CPU Usage"
          :labels="chartLabels"
          :datasets="cpuDatasets"
          y-axis-suffix="%"
          :y-axis-max="100"
          @range-change="onRangeChange" />
      </div>

      <div class="q-mb-md">
        <MetricsLineChart
          title="Memory Usage"
          :labels="chartLabels"
          :datasets="memoryDatasets"
          y-axis-suffix=" MB"
          @range-change="onRangeChange" />
      </div>

      <div>
        <MetricsLineChart
          title="Player Count"
          :labels="chartLabels"
          :datasets="playerDatasets"
          @range-change="onRangeChange" />
      </div>
    </template>
    <q-inner-loading :showing="loading" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { create } from '@bufbuild/protobuf'
import { TimestampSchema, Timestamp } from '@bufbuild/protobuf/wkt'
import { GameServerMetricsHistoryPoint } from 'src/proto/shared_pb'
import { GetGameServerMetricsHistoryRequestSchema } from 'src/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import MetricsLineChart from '@/components/dashboard/MetricsLineChart.vue'

const props = defineProps<{
  gameServerId: string
}>()

const loading = ref(false)
const historyPoints = ref<GameServerMetricsHistoryPoint[]>([])
const selectedRange = ref('1h')

const rangeMs: Record<string, number> = {
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
}

function toTimestamp(d: Date): Timestamp {
  return create(TimestampSchema, {
    seconds: BigInt(Math.floor(d.getTime() / 1000)),
    nanos: 0,
  })
}

const chartLabels = computed(() =>
  historyPoints.value.map((p) => {
    const d = p.timestamp ? new Date(Number(p.timestamp.seconds) * 1000) : new Date()
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }),
)

const cpuDatasets = computed(() => [
  { label: 'CPU %', data: historyPoints.value.map((p) => p.cpuPercent) },
])

// Show memory in MB (bytes / 1024 / 1024)
const memoryDatasets = computed(() => [
  {
    label: 'Memory (MB)',
    data: historyPoints.value.map((p) => Math.round(Number(p.memoryBytes) / (1024 * 1024))),
  },
])

const playerDatasets = computed(() => [
  {
    label: 'Players',
    data: historyPoints.value.map((p) => p.playerCount),
    borderColor: '#26A69A',
  },
])

async function fetchHistory() {
  loading.value = true
  try {
    const now = new Date()
    const since = new Date(now.getTime() - (rangeMs[selectedRange.value] ?? rangeMs['1h']))

    const resp = await GetXylonaClient().getGameServerMetricsHistory(
      create(GetGameServerMetricsHistoryRequestSchema, {
        gameServerId: props.gameServerId,
        since: toTimestamp(since),
        until: toTimestamp(now),
      }),
    )
    historyPoints.value = resp.points
  } catch (err) {
    const connectErr = ConnectError.from(err)
    console.error('Failed to fetch game server metrics history:', connectErr.message)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(connectErr),
      timeout: 5000,
      closeBtn: 'Dismiss',
    })
  } finally {
    loading.value = false
  }
}

function onRangeChange(range: string) {
  selectedRange.value = range
  fetchHistory()
}

onMounted(() => {
  fetchHistory()
})
</script>
