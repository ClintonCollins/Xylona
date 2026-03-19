<template>
  <q-card class="detail-card q-mt-md">
    <q-card-section>
      <div v-if="snapshot" class="q-mb-md">
        <div class="text-subtitle2 q-mb-sm">Resource Usage</div>
        <node-resource-gauges :snapshot="snapshot" />
        <div class="row q-mt-sm q-gutter-md text-caption">
          <div>
            <q-icon name="dns" size="xs" class="q-mr-xs" />
            {{ snapshot.gameServerCount ?? 0 }} servers ({{ snapshot.runningGameServerCount ?? 0 }}
            running)
          </div>
          <div>
            <q-icon name="people" size="xs" class="q-mr-xs" />
            {{ snapshot.userCount ?? 0 }} users
          </div>
        </div>
      </div>

      <div v-if="systemInfo" class="q-mb-md">
        <div class="text-subtitle2 q-mb-sm">System Information</div>
        <q-list separator dense>
          <q-item v-if="systemInfo.cpuModel">
            <q-item-section>CPU</q-item-section>
            <q-item-section side
              >{{ systemInfo.cpuModel }} ({{ systemInfo.cpuCores }}C /
              {{ systemInfo.cpuThreads }}T)</q-item-section
            >
          </q-item>
          <q-item>
            <q-item-section>Total Memory</q-item-section>
            <q-item-section side>{{
              bytesToSize(Number(systemInfo.totalMemoryBytes))
            }}</q-item-section>
          </q-item>
          <q-item>
            <q-item-section>OS</q-item-section>
            <q-item-section side>{{ systemInfo.os }} {{ systemInfo.osVersion }}</q-item-section>
          </q-item>
          <q-item>
            <q-item-section>Architecture</q-item-section>
            <q-item-section side>{{ systemInfo.architecture }}</q-item-section>
          </q-item>
          <q-item>
            <q-item-section>Xylona Version</q-item-section>
            <q-item-section side>{{ systemInfo.xylonaVersion }}</q-item-section>
          </q-item>
        </q-list>
      </div>

      <div class="row q-col-gutter-md">
        <div class="col-12 col-md-6">
          <metrics-line-chart
            title="CPU Usage"
            :labels="chartLabels"
            :datasets="cpuDatasets"
            y-axis-suffix="%"
            :y-axis-max="100"
            @range-change="onRangeChange" />
        </div>
        <div class="col-12 col-md-6">
          <metrics-line-chart
            title="Memory Usage (%)"
            :labels="chartLabels"
            :datasets="memoryPercentDatasets"
            y-axis-suffix="%"
            :y-axis-max="100"
            @range-change="onRangeChange" />
        </div>
        <div class="col-12 col-md-6">
          <metrics-line-chart
            title="Memory Usage (GB)"
            :labels="chartLabels"
            :datasets="memoryBytesDatasets"
            y-axis-suffix=" GB"
            @range-change="onRangeChange" />
        </div>
        <div class="col-12 col-md-6">
          <metrics-line-chart
            title="Disk Usage"
            :labels="chartLabels"
            :datasets="diskDatasets"
            y-axis-suffix="%"
            :y-axis-max="100"
            @range-change="onRangeChange" />
        </div>
      </div>
    </q-card-section>
    <q-inner-loading :showing="loading" />
  </q-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { Timestamp, TimestampSchema } from '@bufbuild/protobuf/wkt'
import { Node, NodeSystemInfo, NodeResourceSnapshot } from 'src/proto/shared_pb'
import {
  GetNodeMetricsHistoryRequestSchema,
  GetNodeSystemInfoRequestSchema,
} from 'src/proto/xylona_pb'
import { AllNodeMetrics } from 'src/proto/websocket_pb'
import { MetricsHistoryPoint } from 'src/proto/shared_pb'
import { GetXylonaClient, bytesToSize, XylonaEventBus } from '@/utils/shared'
import MetricsLineChart from '@/components/shared/MetricsLineChart.vue'
import NodeResourceGauges from '@/components/nodes/NodeResourceGauges.vue'

const props = defineProps<{
  node: Node
  systemInfo?: NodeSystemInfo
  snapshot?: NodeResourceSnapshot
}>()

const loading = ref(false)
const localSystemInfo = ref<NodeSystemInfo | undefined>(props.systemInfo)
const historyPoints = ref<MetricsHistoryPoint[]>([])
const selectedRange = ref('1h')
const liveSnapshot = ref<NodeResourceSnapshot | undefined>(undefined)

const systemInfo = computed(() => localSystemInfo.value)
const snapshot = computed(() => liveSnapshot.value ?? props.snapshot)

function onNodeMetrics(metrics: AllNodeMetrics | undefined) {
  if (!metrics?.nodes) return
  const snap = metrics.nodes[props.node.id]
  if (snap) {
    liveSnapshot.value = snap
  }
}

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

const memoryPercentDatasets = computed(() => [
  { label: 'Memory %', data: historyPoints.value.map((p) => p.memoryPercent) },
])

const memoryBytesDatasets = computed(() => [
  {
    label: 'Memory (GB)',
    data: historyPoints.value.map((p) =>
      parseFloat((Number(p.memoryUsedBytes) / (1024 * 1024 * 1024)).toFixed(2)),
    ),
  },
])

const diskDatasets = computed(() => [
  { label: 'Disk %', data: historyPoints.value.map((p) => p.diskPercent) },
])

async function fetchHistory() {
  loading.value = true
  try {
    const now = new Date()
    const since = new Date(now.getTime() - (rangeMs[selectedRange.value] ?? rangeMs['1h']))

    const resp = await GetXylonaClient().getNodeMetricsHistory(
      create(GetNodeMetricsHistoryRequestSchema, {
        nodeId: props.node.id,
        since: toTimestamp(since),
        until: toTimestamp(now),
      }),
    )
    historyPoints.value = resp.points
  } catch (err) {
    console.error('Failed to fetch node metrics history:', ConnectError.from(err).message)
  } finally {
    loading.value = false
  }
}

async function fetchSystemInfo() {
  if (localSystemInfo.value) return
  try {
    const resp = await GetXylonaClient().getNodeSystemInfo(
      create(GetNodeSystemInfoRequestSchema, { nodeId: props.node.id }),
    )
    localSystemInfo.value = resp.systemInfo
  } catch (err) {
    console.error('Failed to fetch node system info:', ConnectError.from(err).message)
  }
}

function onRangeChange(range: string) {
  selectedRange.value = range
  fetchHistory()
}

onMounted(async () => {
  XylonaEventBus.on('nodeMetrics', onNodeMetrics)
  await Promise.all([fetchSystemInfo(), fetchHistory()])
})

onBeforeUnmount(() => {
  XylonaEventBus.off('nodeMetrics', onNodeMetrics)
})
</script>

<style scoped>
.detail-card {
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
}
</style>
