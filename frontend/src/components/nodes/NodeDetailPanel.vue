<template>
  <div class="node-detail">
    <section aria-label="Node status" class="node-strip">
      <div class="node-strip__item">
        <span>Status</span>
        <strong :class="`node-strip__health--${healthBadge.color}`">
          <q-icon aria-hidden="true" :name="healthBadge.icon" size="16px" />
          {{ healthBadge.label }}
        </strong>
        <small>{{ lastSeenLabel }}</small>
      </div>
      <div class="node-strip__item">
        <span>Mode</span>
        <strong>{{ node.local ? 'Controller host' : 'Remote node' }}</strong>
        <small class="font-mono">{{
          node.local ? 'in-process' : node.baseUrl || 'no address'
        }}</small>
      </div>
      <div class="node-strip__item">
        <span>Agent</span>
        <strong class="font-mono">{{ version.short || 'Unreported' }}</strong>
        <small class="font-mono">{{ version.build ? `build ${version.build}` : os }}</small>
      </div>
      <div class="node-strip__item">
        <span>Servers</span>
        <strong class="font-mono">
          {{ snapshot ? `${snapshot.runningGameServerCount} / ${snapshot.gameServerCount}` : '—' }}
        </strong>
        <small>{{ serversDetail }}</small>
      </div>
      <div class="node-strip__item">
        <span>Live metrics</span>
        <strong :class="liveState.className">
          <span aria-hidden="true" class="node-strip__dot" />
          {{ liveState.label }}
        </strong>
        <small>{{ liveState.detail }}</small>
      </div>
    </section>

    <section aria-label="History controls" class="node-toolbar">
      <q-btn-toggle
        v-model="selectedRange"
        aria-label="Select history time range"
        class="xy-segmented-toggle"
        :options="rangeOptions"
        dense
        no-caps
        toggle-color="primary" />
      <div class="node-toolbar__metadata">
        <span v-if="sampleIntervalSeconds > 0">{{ intervalLabel }}</span>
        <span>{{ samples.length }} points</span>
        <span v-if="historyError" class="node-toolbar__error" role="alert">
          {{ historyError }}
          <q-btn dense flat label="Retry" no-caps size="sm" @click="fetchHistory" />
        </span>
      </div>
    </section>

    <section aria-label="Resource history" class="node-lanes">
      <metric-time-series-chart
        title="CPU"
        description="Host CPU utilisation."
        :empty-label="historyError ? historyFailedLabel : 'No CPU samples in this range.'"
        :bands="cpuBands"
        :format-value="formatPercent"
        :health="cpuHealth"
        :lane-caption="cpuCaption"
        :lane-height="96"
        :samples="samples"
        :series="cpuSeries"
        :summary="cpuSummary"
        variant="lane"
        :y-axis-maximum="100" />
      <metric-time-series-chart
        title="Memory"
        description="Host memory in use."
        :empty-label="historyError ? historyFailedLabel : 'No memory samples in this range.'"
        :bands="memoryBands"
        :format-value="formatBytes"
        :health="memoryHealth"
        :lane-caption="memoryCaption"
        :lane-height="96"
        :samples="samples"
        :series="memorySeries"
        :summary="memorySummary"
        variant="lane"
        :y-axis-maximum="memoryTotalBytes ?? undefined" />
      <metric-time-series-chart
        title="Disk"
        description="Used space on the node's install volume."
        :empty-label="historyError ? historyFailedLabel : 'No disk samples in this range.'"
        :bands="diskBands"
        :format-value="formatBytes"
        :health="diskHealth"
        :lane-caption="diskCaption"
        :lane-height="96"
        :lane-note="diskProjection"
        :samples="samples"
        :series="diskSeries"
        :summary="diskSummary"
        variant="lane"
        :y-axis-maximum="diskTotalBytes ?? undefined" />
      <metric-time-series-chart
        title="Servers"
        description="Running game servers against the total assigned to this node."
        :empty-label="historyError ? historyFailedLabel : 'No server counts in this range.'"
        :format-value="formatWhole"
        lane-caption="running / assigned"
        :lane-height="64"
        :samples="samples"
        :series="serverSeries"
        :summary="serverSummary"
        variant="lane" />
      <q-inner-loading :showing="historyLoading && samples.length === 0" />
    </section>

    <section aria-labelledby="node-servers-title" class="node-section">
      <h2 id="node-servers-title" class="node-section__title">Servers on this node</h2>
      <q-table
        aria-label="Game servers on this node"
        class="xy-standalone-table"
        :columns="serverColumns"
        dense
        flat
        hide-pagination
        :loading="serversLoading"
        :rows="nodeServers"
        :rows-per-page-options="[0]"
        row-key="id"
        :grid="$q.screen.lt.sm">
        <template #body-cell-name="cell">
          <q-td :props="cell">
            <router-link class="table-link" :to="`/game-servers/${cell.row.id}`">
              {{ cell.row.name }}
            </router-link>
            <span class="text-caption text-xy-muted q-ml-sm">{{ cell.row.gameName }}</span>
          </q-td>
        </template>
        <!-- Phones get one compact row per server instead of a sideways-scrolling table. -->
        <template #item="item">
          <div class="node-server-item col-12">
            <div class="node-server-item__head">
              <router-link class="table-link" :to="`/game-servers/${item.row.id}`">
                {{ item.row.name }}
              </router-link>
              <status-badge :status="serverStatus(item.row)" />
            </div>
            <div class="node-server-item__meta">
              <span>{{ item.row.gameName }}</span>
              <span class="font-mono">
                CPU {{ serverCpu(item.row) }} · Mem {{ serverMemory(item.row) }} · Up
                {{ serverUptime(item.row) }}
              </span>
            </div>
          </div>
        </template>
        <template #body-cell-status="cell">
          <q-td :props="cell">
            <status-badge :status="serverStatus(cell.row)" />
          </q-td>
        </template>
        <template #body-cell-cpu="cell">
          <q-td :props="cell">
            <span class="font-mono">{{ serverCpu(cell.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-memory="cell">
          <q-td :props="cell">
            <span class="font-mono">{{ serverMemory(cell.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-uptime="cell">
          <q-td :props="cell">
            <span class="font-mono">{{ serverUptime(cell.row) }}</span>
          </q-td>
        </template>
        <template #no-data>
          <div class="node-section__empty">No game servers are assigned to this node.</div>
        </template>
      </q-table>
    </section>

    <section aria-labelledby="node-system-title" class="node-section">
      <h2 id="node-system-title" class="node-section__title">System</h2>
      <dl v-if="currentSystemInfo" class="node-system">
        <div>
          <dt>CPU</dt>
          <dd>
            {{ currentSystemInfo.cpuModel || 'Unknown' }}
            <span v-if="currentSystemInfo.cpuThreads" class="font-mono text-xy-muted">
              {{ currentSystemInfo.cpuCores }}C / {{ currentSystemInfo.cpuThreads }}T
            </span>
          </dd>
        </div>
        <div>
          <dt>Memory</dt>
          <dd class="font-mono">{{ formatBytes(Number(currentSystemInfo.totalMemoryBytes)) }}</dd>
        </div>
        <div>
          <dt>Disk</dt>
          <dd class="font-mono">{{ diskTotalBytes ? formatBytes(diskTotalBytes) : 'Unknown' }}</dd>
        </div>
        <div>
          <dt>OS</dt>
          <dd>{{ currentSystemInfo.os }} {{ currentSystemInfo.osVersion }}</dd>
        </div>
        <div>
          <dt>Architecture</dt>
          <dd class="font-mono">{{ currentSystemInfo.architecture }}</dd>
        </div>
        <div>
          <dt>Agent version</dt>
          <dd class="font-mono">{{ currentSystemInfo.xylonaVersion || 'Unreported' }}</dd>
        </div>
      </dl>
      <div v-else class="node-section__empty">
        System information arrives with the node's first snapshot.
      </div>
    </section>
  </div>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import { ConnectError } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { GameServer, Node, NodeResourceSnapshot, NodeSystemInfo, Status } from '@/proto/shared_pb'
import {
  GetNodeMetricsHistoryRequestSchema,
  GetNodeSystemInfoRequestSchema,
  ListGameServersRequestSchema,
} from '@/proto/xylona_pb'
import { AllNodeMetrics, AllServersMetrics, GameServerMetrics } from '@/proto/websocket_pb'
import { ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { createServerMetricsSubscriptions } from '@/utils/server-metrics-subscriptions'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'
import {
  getMetricsRangeOption,
  getMetricsRangeRequest,
  LatestRequestGuard,
  metricsRangeOptions,
  type MetricHealth,
  type MetricsRangeKey,
  type MetricSummary,
} from '@/pages/game_servers/game-server-metrics'
import {
  formatMetricAge,
  formatMetricBytes,
  formatMetricPercent,
} from '@/pages/game_servers/metrics-format'
import MetricTimeSeriesChart, {
  type MetricChartBand,
  type MetricChartSeries,
} from '@/components/game_servers/MetricTimeSeriesChart.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import {
  nodeHealthBadge,
  nodeLastSeenMs,
  nodeResourceHealth,
  nodeResourceThresholds,
  projectDaysUntilDiskFull,
  splitNodeVersion,
  type NodeResource,
} from './node-display'

interface NodeSample {
  timestampMs: number
  cpuPercent: number
  memoryPercent: number
  memoryUsedBytes: number
  diskPercent: number
  diskUsedBytes: number
  gameServerCount: number | null
  runningGameServerCount: number | null
}

const props = defineProps<{
  node: Node
  systemInfo?: NodeSystemInfo
  snapshot?: NodeResourceSnapshot
}>()

const rangeOptions = metricsRangeOptions.filter((option) => option.value !== '90d')
const defaultRange = getMetricsRangeOption('1h')
const selectedRange = ref<MetricsRangeKey>('1h')
const currentRange = computed(
  () => rangeOptions.find((option) => option.value === selectedRange.value) ?? defaultRange,
)

const $q = useQuasar()
const historyLoading = ref(false)
const historyError = ref('')
const historyFailedLabel = 'History could not be loaded for this range.'
const historySamples = ref<NodeSample[]>([])
// Disk growth is projected from its own week of history, whatever range is shown.
const diskTrendSamples = ref<NodeSample[]>([])
const liveTail = ref<NodeSample[]>([])
const sampleIntervalSeconds = ref(0)
const historyGuard = new LatestRequestGuard()

const localSystemInfo = ref<NodeSystemInfo | undefined>(props.systemInfo)
const liveSnapshot = ref<NodeResourceSnapshot | undefined>(undefined)
const liveSnapshotAtMs = ref<number | null>(null)
const nowMs = ref(Date.now())
let clockTimer: ReturnType<typeof setInterval> | null = null

const allServers = ref<GameServer[]>([])
const serversLoading = ref(false)
const metricsSubscriptions = createServerMetricsSubscriptions()
const serverMetrics = ref<Map<string, GameServerMetrics>>(new Map())
const serverStatuses = ref<Map<string, Status>>(new Map())

const currentSystemInfo = computed(() => localSystemInfo.value ?? props.systemInfo)
const snapshot = computed(() => liveSnapshot.value ?? props.snapshot)
const healthBadge = computed(() => nodeHealthBadge(props.node))
const version = computed(() =>
  splitNodeVersion(currentSystemInfo.value?.xylonaVersion || props.node.version),
)
const os = computed(() => currentSystemInfo.value?.os || props.node.os || '')

const serversDetail = computed(() => {
  const snap = snapshot.value
  if (!snap) return 'no snapshot yet'
  return `running · ${snap.userCount} ${snap.userCount === 1 ? 'user' : 'users'}`
})

const lastSeenLabel = computed(() => {
  const seenMs = nodeLastSeenMs(props.node)
  if (liveSnapshotAtMs.value !== null)
    return `seen ${formatMetricAge(liveSnapshotAtMs.value, nowMs.value)}`
  return seenMs === null ? 'never seen' : `seen ${formatMetricAge(seenMs, nowMs.value)}`
})

const liveState = computed(() => {
  if (!websocketStateAuthoritative.value) {
    return {
      label: 'Paused',
      detail: 'reconnecting to controller',
      className: 'node-strip__live--paused',
    }
  }
  if (liveSnapshotAtMs.value === null) {
    return {
      label: 'Waiting',
      detail: 'first snapshot pending',
      className: 'node-strip__live--waiting',
    }
  }
  return {
    label: 'Streaming',
    detail: `updated ${formatMetricAge(liveSnapshotAtMs.value, nowMs.value)}`,
    className: 'node-strip__live--on',
  }
})

const memoryTotalBytes = computed(() => {
  const fromSnapshot = Number(snapshot.value?.memoryTotalBytes ?? 0)
  if (fromSnapshot > 0) return fromSnapshot
  const fromInfo = Number(currentSystemInfo.value?.totalMemoryBytes ?? 0)
  return fromInfo > 0 ? fromInfo : null
})
const diskTotalBytes = computed(() => {
  const total = Number(snapshot.value?.diskTotalBytes ?? 0)
  return total > 0 ? total : null
})

const samples = computed<NodeSample[]>(() => {
  const history = historySamples.value
  const lastHistoryMs = history.at(-1)?.timestampMs ?? 0
  if (!currentRange.value.live) return history
  return history.concat(liveTail.value.filter((sample) => sample.timestampMs > lastHistoryMs))
})

function summarize(select: (sample: NodeSample) => number | null): MetricSummary {
  let minimum: number | null = null
  let maximum: number | null = null
  let total = 0
  let count = 0
  let latest: number | null = null
  for (const sample of samples.value) {
    const value = select(sample)
    if (value === null || !Number.isFinite(value)) continue
    minimum = minimum === null ? value : Math.min(minimum, value)
    maximum = maximum === null ? value : Math.max(maximum, value)
    total += value
    count += 1
    latest = value
  }
  return {
    latest,
    minimum,
    maximum,
    average: count > 0 ? total / count : null,
    coverageRatio: null,
    sampleCount: samples.value.length,
  }
}

const cpuSeries: MetricChartSeries<NodeSample>[] = [
  { label: 'CPU', colorToken: '--xy-series-1', value: (sample) => sample.cpuPercent },
]
const memorySeries: MetricChartSeries<NodeSample>[] = [
  { label: 'Used', colorToken: '--xy-series-2', value: (sample) => sample.memoryUsedBytes },
]
const diskSeries: MetricChartSeries<NodeSample>[] = [
  { label: 'Used', colorToken: '--xy-series-3', value: (sample) => sample.diskUsedBytes },
]
const serverSeries: MetricChartSeries<NodeSample>[] = [
  {
    label: 'Running',
    colorToken: '--xy-series-1',
    value: (sample) => sample.runningGameServerCount,
  },
  {
    label: 'Assigned',
    colorToken: '--xy-series-neutral',
    value: (sample) => sample.gameServerCount,
    dashed: true,
  },
]

const cpuSummary = computed(() => summarize((sample) => sample.cpuPercent))
const memorySummary = computed(() => summarize((sample) => sample.memoryUsedBytes))
const diskSummary = computed(() => summarize((sample) => sample.diskUsedBytes))
const serverSummary = computed(() => summarize((sample) => sample.runningGameServerCount))

function thresholdBands(resource: NodeResource, total: number | null): MetricChartBand[] {
  if (!total) return []
  const { warn, danger } = nodeResourceThresholds[resource]
  return [
    { from: (total * warn) / 100, to: (total * danger) / 100 },
    { from: (total * danger) / 100, colorToken: '--xy-danger-bg-faint' },
  ]
}

const cpuBands = thresholdBands('cpu', 100)
const memoryBands = computed(() => thresholdBands('memory', memoryTotalBytes.value))
const diskBands = computed(() => thresholdBands('disk', diskTotalBytes.value))

function resourceHealth(resource: NodeResource, percent: number | undefined): MetricHealth {
  const { level, label } = nodeResourceHealth(resource, percent)
  return { level, label }
}

const cpuHealth = computed(() => resourceHealth('cpu', snapshot.value?.cpuPercent))
const memoryHealth = computed(() => resourceHealth('memory', snapshot.value?.memoryPercent))
const diskHealth = computed(() => resourceHealth('disk', snapshot.value?.diskPercent))

const cpuCaption = computed(() => {
  const threads = currentSystemInfo.value?.cpuThreads
  const percent = snapshot.value?.cpuPercent
  if (!threads || percent === undefined) return ''
  return `≈ ${((percent / 100) * threads).toFixed(1)} of ${threads} threads`
})
function capacityCaption(percent: number | undefined, total: number | null): string {
  if (!total) return ''
  const share = percent === undefined ? '' : `${formatMetricPercent(percent, 0)} of `
  return `${share}${formatMetricBytes(total)}`
}

const memoryCaption = computed(() =>
  capacityCaption(snapshot.value?.memoryPercent, memoryTotalBytes.value),
)
const diskCaption = computed(() =>
  capacityCaption(snapshot.value?.diskPercent, diskTotalBytes.value),
)

const diskProjection = computed(() => {
  const days = projectDaysUntilDiskFull(diskTrendSamples.value, diskTotalBytes.value)
  if (days === null) return ''
  return `Full in ~${days < 1 ? '<1' : Math.round(days)} d at this week's rate`
})

const intervalLabel = computed(() => {
  const seconds = sampleIntervalSeconds.value
  if (seconds >= 3600) return `${(seconds / 3600).toFixed(seconds % 3600 === 0 ? 0 : 1)}h buckets`
  if (seconds >= 60) return `${Math.round(seconds / 60)}m buckets`
  return `${seconds}s buckets`
})

function formatPercent(value: number | null): string {
  return formatMetricPercent(value, 0)
}
function formatBytes(value: number | null): string {
  return formatMetricBytes(value)
}
function formatWhole(value: number | null): string {
  return value === null || !Number.isFinite(value) ? 'Unknown' : String(Math.round(value))
}

function snapshotToSample(snap: NodeResourceSnapshot, timestampMs: number): NodeSample {
  return {
    timestampMs,
    cpuPercent: snap.cpuPercent,
    memoryPercent: snap.memoryPercent,
    memoryUsedBytes: Number(snap.memoryUsedBytes),
    diskPercent: snap.diskPercent,
    diskUsedBytes: Number(snap.diskUsedBytes),
    gameServerCount: snap.gameServerCount,
    runningGameServerCount: snap.runningGameServerCount,
  }
}

function onNodeMetrics(metrics: AllNodeMetrics | undefined) {
  const snap = metrics?.nodes[props.node.id]
  if (!snap) return
  const atMs = snap.recordedAt?.seconds ? Number(snap.recordedAt.seconds) * 1000 : Date.now()
  liveSnapshot.value = snap
  liveSnapshotAtMs.value = atMs
  const tail = liveTail.value.filter(
    (sample) => sample.timestampMs >= atMs - currentRange.value.durationMs,
  )
  tail.push(snapshotToSample(snap, atMs))
  liveTail.value = tail
}

function onServerMetrics(metrics: AllServersMetrics | undefined) {
  if (!metrics?.servers) return
  const next = new Map(serverMetrics.value)
  for (const [id, metric] of Object.entries(metrics.servers)) next.set(id, metric)
  serverMetrics.value = next
}

function onServerStatus(gameServerId: string, _name: string, status: Status) {
  const next = new Map(serverStatuses.value)
  next.set(gameServerId, status)
  serverStatuses.value = next
}

const nodeServers = computed(() =>
  allServers.value.filter((server) => server.nodeId === props.node.id),
)

function serverStatus(server: GameServer): Status {
  return serverStatuses.value.get(server.id) ?? server.status
}
function serverCpu(server: GameServer): string {
  const metric = serverMetrics.value.get(server.id)
  return metric?.metricsValid && metric.cpuValid ? formatMetricPercent(metric.cpuPercent) : '—'
}
function serverMemoryBytes(metric: GameServerMetrics | undefined): number | null {
  if (!metric?.metricsValid) return null
  const workingSet = Number(metric.memoryWorkingSetBytes)
  return workingSet > 0 ? workingSet : Number(metric.memoryBytes)
}
function serverMemory(server: GameServer): string {
  const bytes = serverMemoryBytes(serverMetrics.value.get(server.id))
  return bytes === null ? '—' : formatMetricBytes(bytes)
}
function serverUptime(server: GameServer): string {
  const metric = serverMetrics.value.get(server.id)
  const seconds = Number(metric?.uptimeSeconds ?? 0)
  if (!metric?.metricsValid || seconds <= 0) return '—'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}

const serverColumns = [
  {
    name: 'name',
    label: 'Server',
    align: 'left' as const,
    field: (row: GameServer) => row.name,
    sortable: true,
  },
  {
    name: 'status',
    label: 'Status',
    align: 'left' as const,
    field: (row: GameServer) => serverStatus(row),
    sortable: true,
  },
  {
    name: 'cpu',
    label: 'CPU',
    align: 'right' as const,
    field: (row: GameServer) => serverMetrics.value.get(row.id)?.cpuPercent ?? -1,
    sortable: true,
  },
  {
    name: 'memory',
    label: 'Memory',
    align: 'right' as const,
    field: (row: GameServer) => serverMemoryBytes(serverMetrics.value.get(row.id)) ?? -1,
    sortable: true,
  },
  {
    name: 'uptime',
    label: 'Uptime',
    align: 'right' as const,
    field: (row: GameServer) => Number(serverMetrics.value.get(row.id)?.uptimeSeconds ?? -1),
    sortable: true,
  },
]

async function loadHistory(sinceMs: number, untilMs: number, maxPoints: number) {
  const resp = await GetXylonaClient().getNodeMetricsHistory(
    create(GetNodeMetricsHistoryRequestSchema, {
      nodeId: props.node.id,
      since: create(TimestampSchema, { seconds: BigInt(Math.floor(sinceMs / 1000)) }),
      until: create(TimestampSchema, { seconds: BigInt(Math.floor(untilMs / 1000)) }),
      maxPoints,
    }),
  )
  const points: NodeSample[] = resp.points.map((point) => ({
    timestampMs: Number(point.timestamp?.seconds ?? 0n) * 1000,
    cpuPercent: point.cpuPercent,
    memoryPercent: point.memoryPercent,
    memoryUsedBytes: Number(point.memoryUsedBytes),
    diskPercent: point.diskPercent,
    diskUsedBytes: Number(point.diskUsedBytes),
    // Rows recorded before counts were stored come back as 0/0; treat that as unknown.
    gameServerCount: point.gameServerCount > 0 ? point.gameServerCount : null,
    runningGameServerCount: point.gameServerCount > 0 ? point.runningGameServerCount : null,
  }))
  return { points, sampleIntervalSeconds: resp.sampleIntervalSeconds }
}

async function fetchHistory() {
  const sequence = historyGuard.begin()
  historyLoading.value = true
  historyError.value = ''
  try {
    const range = getMetricsRangeRequest(selectedRange.value)
    const history = await loadHistory(range.sinceMs, range.untilMs, range.maxPoints)
    if (!historyGuard.isCurrent(sequence)) return
    historySamples.value = history.points
    sampleIntervalSeconds.value = history.sampleIntervalSeconds
  } catch (err) {
    if (!historyGuard.isCurrent(sequence)) return
    // Never leave the previous range drawn under the new selection.
    historySamples.value = []
    sampleIntervalSeconds.value = 0
    historyError.value = ConnectErrorToString(ConnectError.from(err))
  } finally {
    if (historyGuard.isCurrent(sequence)) historyLoading.value = false
  }
}

async function fetchDiskTrend() {
  const untilMs = Date.now()
  try {
    const history = await loadHistory(untilMs - 7 * 24 * 60 * 60 * 1000, untilMs, 168)
    diskTrendSamples.value = history.points
  } catch (err) {
    console.error('Failed to fetch node disk trend:', ConnectError.from(err).message)
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

async function fetchServers() {
  serversLoading.value = true
  try {
    const resp = await GetXylonaClient().listGameServers(create(ListGameServersRequestSchema, {}))
    allServers.value = resp.gameServers
  } catch (err) {
    console.error('Failed to list game servers for node:', ConnectError.from(err).message)
  } finally {
    serversLoading.value = false
  }
}

watch(selectedRange, () => void fetchHistory())

// The table's CPU, memory and uptime columns only fill while subscribed to each server.
function syncServerSubscriptions() {
  metricsSubscriptions.sync(nodeServers.value.map((server) => server.id))
}
watch(nodeServers, syncServerSubscriptions)

function onWebsocketConnected() {
  metricsSubscriptions.forget()
  syncServerSubscriptions()
}

onMounted(async () => {
  XylonaEventBus.on('nodeMetrics', onNodeMetrics)
  XylonaEventBus.on('gameServerMetrics', onServerMetrics)
  XylonaEventBus.on('gameServerStatus', onServerStatus)
  XylonaEventBus.on('websocketConnected', onWebsocketConnected)
  XylonaEventBus.on('websocketDisconnected', metricsSubscriptions.forget)
  clockTimer = setInterval(() => (nowMs.value = Date.now()), 5000)
  await Promise.all([fetchSystemInfo(), fetchHistory(), fetchDiskTrend(), fetchServers()])
})

onBeforeUnmount(() => {
  historyGuard.invalidate()
  XylonaEventBus.off('nodeMetrics', onNodeMetrics)
  XylonaEventBus.off('gameServerMetrics', onServerMetrics)
  XylonaEventBus.off('gameServerStatus', onServerStatus)
  XylonaEventBus.off('websocketConnected', onWebsocketConnected)
  XylonaEventBus.off('websocketDisconnected', metricsSubscriptions.forget)
  metricsSubscriptions.clear()
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<style scoped>
.node-detail {
  display: grid;
  gap: var(--xy-space-lg);
}

.node-strip {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  background: var(--xy-surface-1);
  border-top: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
}

.node-strip__item {
  display: grid;
  gap: var(--xy-space-2xs);
  min-width: 0;
  padding: var(--xy-space-base) var(--xy-space-md);
  color: var(--xy-text-primary);
}

.node-strip__item + .node-strip__item {
  border-left: 1px solid var(--xy-border);
}

.node-strip__item > span,
.node-strip__item > small {
  overflow: hidden;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-strip__item > span {
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.node-strip__item > strong {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  overflow: hidden;
  font-size: var(--xy-font-size-base);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-strip__health--positive {
  color: var(--xy-success);
}

.node-strip__health--warning {
  color: var(--xy-warning);
}

.node-strip__health--negative {
  color: var(--xy-danger);
}

.node-strip__dot {
  width: 8px;
  height: 8px;
  background: currentcolor;
  border-radius: var(--xy-radius-pill);
}

.node-strip__live--on {
  color: var(--xy-success-text-soft);
}

.node-strip__live--waiting {
  color: var(--xy-text-secondary);
}

.node-strip__live--paused {
  color: var(--xy-warning);
}

.node-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.node-toolbar__metadata {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.node-toolbar__error {
  color: var(--xy-danger);
}

.node-lanes {
  --metric-lane-gutter: 224px;

  position: relative;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  overflow: hidden;
}

.node-lanes > .metric-chart--lane + .metric-chart--lane {
  border-top: 1px solid var(--xy-border);
}

.node-section {
  display: grid;
  gap: var(--xy-space-sm);
}

.node-section__title {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 700;
  letter-spacing: 0.02em;
}

.node-server-item {
  display: grid;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.node-server-item__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.node-server-item__meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.node-section__empty {
  padding: var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}

.node-system {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--xy-space-md) var(--xy-space-lg);
  margin: 0;
  padding: var(--xy-space-md);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.node-system > div {
  min-width: 0;
}

.node-system dt {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.node-system dd {
  margin: var(--xy-space-2xs) 0 0;
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

@media (max-width: 1023px) {
  .node-strip {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .node-strip__item:nth-child(3n + 1) {
    border-left: 0;
  }

  .node-strip__item:nth-child(n + 4) {
    border-top: 1px solid var(--xy-border);
  }

  .node-lanes {
    --metric-lane-gutter: 160px;
  }

  .node-system {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 599px) {
  .node-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .node-strip__item:nth-child(n) {
    border-top: 0;
    border-left: 0;
  }

  .node-strip__item:nth-child(even) {
    border-left: 1px solid var(--xy-border);
  }

  .node-strip__item:nth-child(n + 3) {
    border-top: 1px solid var(--xy-border);
  }

  .node-strip__item:last-child:nth-child(odd) {
    grid-column: 1 / -1;
  }

  .node-system {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
