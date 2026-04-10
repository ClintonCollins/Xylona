<template>
  <div>
    <div v-if="!loading && historyPoints.length === 0" class="empty-state">
      <q-icon class="text-xy-muted" name="show_chart" size="48px" />
      <div class="text-subtitle1 q-mt-sm text-xy-secondary">No Metrics Data</div>
      <div class="text-caption text-xy-muted">
        Metrics are recorded every 60 seconds while the server is running.
      </div>
    </div>
    <div v-if="historyPoints.length > 0" class="row q-col-gutter-md">
      <div class="col-12 col-md-6">
        <metrics-line-chart
          :datasets="cpuDatasets"
          :labels="chartLabels"
          :y-axis-max="100"
          title="CPU Usage"
          y-axis-suffix="%"
          @range-change="onRangeChange" />
      </div>
      <div class="col-12 col-md-6">
        <metrics-line-chart
          :datasets="memoryDatasets"
          :labels="chartLabels"
          title="Memory Usage"
          y-axis-suffix=" MB"
          @range-change="onRangeChange" />
      </div>
      <div class="col-12 col-md-6">
        <metrics-line-chart
          :datasets="playerDatasets"
          :labels="chartLabels"
          title="Player Count"
          @range-change="onRangeChange" />
      </div>
    </div>
    <q-inner-loading :showing="loading" />
  </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { create } from '@bufbuild/protobuf'
import { Timestamp, TimestampSchema } from '@bufbuild/protobuf/wkt'
import { GameServerMetricsHistoryPoint } from '@/proto/shared_pb'
import { GetGameServerMetricsHistoryRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import MetricsLineChart from '@/components/shared/MetricsLineChart.vue'

function getCssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

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
    borderColor: getCssVar('--xy-chart-2'),
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
  void fetchHistory()
}

onMounted(() => {
  void fetchHistory()
})
</script>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--xy-space-2xl) var(--xy-space-md);
}
</style>
