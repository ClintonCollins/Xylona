<template>
  <q-page class="xy-page-content">
    <div v-if="!detailNodeId">
      <page-header title="Nodes">
        <div class="text-caption text-xy-secondary">
          {{ rows.length }} {{ rows.length === 1 ? 'node' : 'nodes' }}
          <template v-if="totalServers > 0">
            &middot; {{ totalServers }} {{ totalServers === 1 ? 'server' : 'servers' }}
            <span v-if="runningServers > 0" class="text-success"
              >({{ runningServers }} running)</span
            >
          </template>
          <template v-if="totalUsers > 0">
            &middot; {{ totalUsers }} {{ totalUsers === 1 ? 'user' : 'users' }}
          </template>
        </div>
        <template #actions>
          <q-input
            v-model="search"
            aria-label="Search nodes"
            class="xy-search-input"
            color="primary"
            debounce="300"
            dense
            outlined
            placeholder="Search nodes">
            <template #append>
              <q-icon name="search" />
            </template>
          </q-input>
          <q-btn color="primary" label="Add node" to="/nodes/add" />
        </template>
      </page-header>
      <div v-if="loadError" class="list-error" role="alert" aria-live="assertive">
        <q-icon name="sync_problem" size="sm" />
        <div>
          <strong>Nodes could not be loaded.</strong>
          <span>{{ loadError }}</span>
        </div>
        <q-btn :loading="loading" dense flat icon="refresh" label="Retry" @click="fetchAll" />
      </div>
      <div
        v-if="!websocketStateAuthoritative && !websocketConnectingQuietly"
        class="list-notice"
        role="status"
        aria-live="polite">
        <q-icon name="sync" size="sm" />
        <span>Live metrics are paused while the controller connection is re-established.</span>
      </div>
      <div>
        <q-table
          v-model:pagination="initialPagination"
          aria-label="Nodes"
          :columns="columns"
          :filter="search"
          :grid="$q.screen.lt.md"
          :loading="loading"
          :rows="rows"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #item="props">
            <div class="node-grid-item col-12 col-sm-6">
              <q-card class="node-mobile-card" flat>
                <q-card-section class="node-mobile-header">
                  <button class="node-mobile-name" type="button" @click="openDetail(props.row)">
                    {{ props.row.name || 'Unnamed node' }}
                  </button>
                  <q-badge
                    :color="nodeHealthBadge(props.row).color"
                    :label="nodeHealthBadge(props.row).label" />
                </q-card-section>

                <q-card-section class="node-mobile-metrics">
                  <div v-for="metric in mobileMetrics" :key="metric.resource">
                    <span>{{ metric.label }}</span>
                    <strong
                      :class="
                        metricClass(metric.resource, resourcePercent(props.row.id, metric.resource))
                      ">
                      <node-metric-unavailable
                        v-if="resourcePercent(props.row.id, metric.resource) === null"
                        :resource="metric.resource" />
                      <template v-else>
                        {{
                          formatMetric(
                            resourcePercent(props.row.id, metric.resource),
                            metric.resource,
                          )
                        }}
                      </template>
                    </strong>
                  </div>
                  <div>
                    <span>Servers</span>
                    <strong>
                      {{ getSnapshot(props.row.id)?.runningGameServerCount ?? '—' }} /
                      {{ getSnapshot(props.row.id)?.gameServerCount ?? '—' }}
                    </strong>
                  </div>
                </q-card-section>

                <q-card-section class="node-mobile-meta">
                  <span :title="getNodeVersion(props.row.id)"
                    >Version
                    <span class="font-mono">{{
                      splitNodeVersion(getNodeVersion(props.row.id)).short || 'not reported'
                    }}</span></span
                  >
                  <span :title="lastSeenAbsolute(props.row)">
                    Last seen {{ lastSeenRelative(props.row) }}
                  </span>
                </q-card-section>

                <q-card-actions class="node-mobile-actions">
                  <q-btn
                    flat
                    icon="monitor_heart"
                    label="Details"
                    no-caps
                    @click="openDetail(props.row)" />
                  <q-space />
                  <q-btn
                    :to="`/nodes/${props.row.id}/edit`"
                    :aria-label="`Edit ${props.row.name || 'node'}`"
                    flat
                    icon="edit">
                    <q-tooltip>Edit node</q-tooltip>
                  </q-btn>
                  <q-btn
                    :to="{ path: '/admin/updates', query: { nodeId: props.row.id } }"
                    :aria-label="`Check ${props.row.name || 'node'} for updates`"
                    flat
                    icon="system_update_alt">
                    <q-tooltip>Check for updates</q-tooltip>
                  </q-btn>
                  <q-btn
                    v-if="!props.row.local"
                    :aria-label="`Remove ${props.row.name || 'node'}`"
                    class="text-error-brighter"
                    flat
                    icon="delete"
                    @click="deleteNodeAction(props.row)">
                    <q-tooltip>Remove node</q-tooltip>
                  </q-btn>
                </q-card-actions>
              </q-card>
            </div>
          </template>
          <template #body-cell-name="props">
            <q-td :props="props">
              <button class="table-link" type="button" @click="openDetail(props.row)">
                {{ props.row.name || 'Unnamed' }}
              </button>
            </q-td>
          </template>
          <template #body-cell-health="props">
            <q-td :props="props">
              <q-badge
                :color="nodeHealthBadge(props.row).color"
                :label="nodeHealthBadge(props.row).label" />
            </q-td>
          </template>
          <template #body-cell-cpu="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="3rem" />
              </template>
              <node-metric-unavailable
                v-else-if="resourcePercent(props.row.id, 'cpu') === null"
                resource="cpu" />
              <template v-else-if="getSnapshot(props.row.id)">
                <span
                  :class="metricClass('cpu', resourcePercent(props.row.id, 'cpu'))"
                  class="font-mono">
                  {{ formatMetric(resourcePercent(props.row.id, 'cpu'), 'cpu') }}
                </span>
              </template>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-ram="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="5rem" />
              </template>
              <node-metric-unavailable
                v-else-if="resourcePercent(props.row.id, 'memory') === null"
                resource="memory" />
              <template v-else-if="getSnapshot(props.row.id)">
                <span
                  :class="metricClass('memory', resourcePercent(props.row.id, 'memory'))"
                  class="font-mono">
                  {{ formatMetric(resourcePercent(props.row.id, 'memory'), 'memory') }}
                </span>
                <span class="text-caption text-xy-muted xy-num q-ml-xs">
                  {{ bytesToSize(Number(getSnapshot(props.row.id)!.memoryUsedBytes)) }}
                </span>
              </template>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-disk="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="5rem" />
              </template>
              <node-metric-unavailable
                v-else-if="resourcePercent(props.row.id, 'disk') === null"
                resource="disk" />
              <template v-else-if="getSnapshot(props.row.id)">
                <span
                  :class="metricClass('disk', resourcePercent(props.row.id, 'disk'))"
                  class="font-mono">
                  {{ formatMetric(resourcePercent(props.row.id, 'disk'), 'disk') }}
                </span>
                <span class="text-caption text-xy-muted xy-num q-ml-xs">
                  {{ bytesToSize(Number(getSnapshot(props.row.id)!.diskUsedBytes)) }}
                </span>
              </template>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-servers="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="4rem" />
              </template>
              <span v-else-if="getSnapshot(props.row.id)" class="xy-num">
                <span class="text-success">{{
                  getSnapshot(props.row.id)!.runningGameServerCount
                }}</span>
                /
                {{ getSnapshot(props.row.id)!.gameServerCount }}
              </span>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-version="props">
            <q-td :props="props">
              <q-skeleton
                v-if="shouldShowVersionSkeleton(props.row.id)"
                class="node-list__metric-skeleton"
                type="text"
                width="4rem" />
              <span
                v-else-if="getNodeVersion(props.row.id)"
                :title="getNodeVersion(props.row.id)"
                class="node-version">
                {{ splitNodeVersion(getNodeVersion(props.row.id)).short }}
                <small v-if="splitNodeVersion(getNodeVersion(props.row.id)).build">
                  {{ splitNodeVersion(getNodeVersion(props.row.id)).build }}
                </small>
              </span>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-lastSync="props">
            <q-td :props="props">
              <span v-if="nodeLastSeenMs(props.row) !== null" :title="lastSeenAbsolute(props.row)">
                {{ lastSeenRelative(props.row) }}
              </span>
              <span v-else class="text-xy-muted">Never</span>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="xy-row-actions">
                <q-btn
                  :to="'/nodes/' + props.row.id + '/edit'"
                  :aria-label="`Edit ${props.row.name || 'node'}`"
                  dense
                  flat
                  icon="edit"
                  round>
                  <q-tooltip>Edit node</q-tooltip>
                </q-btn>
                <q-btn
                  :to="{ path: '/admin/updates', query: { nodeId: props.row.id } }"
                  :aria-label="`Check ${props.row.name || 'node'} for updates`"
                  dense
                  flat
                  icon="system_update_alt"
                  round>
                  <q-tooltip>Check for updates</q-tooltip>
                </q-btn>
                <q-btn
                  v-if="!props.row.local"
                  :aria-label="`Remove ${props.row.name || 'node'}`"
                  class="text-error-brighter"
                  dense
                  flat
                  icon="delete"
                  round
                  @click="deleteNodeAction(props.row)">
                  <q-tooltip>Remove node</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <empty-state
              v-if="!loading && !loadError"
              :description="
                search
                  ? 'Try a different search.'
                  : 'Add a remote node to start managing another host.'
              "
              :title="search ? 'No matching nodes' : 'No nodes yet'"
              icon="dns">
              <template v-if="!search" #actions>
                <q-btn color="primary" label="Add node" to="/nodes/add" />
              </template>
            </empty-state>
          </template>
        </q-table>
      </div>
    </div>

    <div v-if="detailNodeId">
      <div class="xy-page-header">
        <div class="row items-center">
          <q-btn aria-label="Back to nodes" dense flat icon="arrow_back" round to="/nodes" />
          <h1 class="xy-page-title q-ml-sm">
            {{ detailNode?.name || (loading ? 'Loading node…' : 'Node not found') }}
          </h1>
        </div>
        <div v-if="detailNode" class="xy-page-actions">
          <q-btn dense flat icon="add_alert" label="Add alert" @click="showAlertDialog = true" />
          <q-btn
            :to="{ path: '/admin/updates', query: { nodeId: detailNode.id } }"
            dense
            flat
            icon="system_update_alt"
            label="Updates" />
          <q-btn :to="'/nodes/' + detailNode.id + '/edit'" dense flat icon="edit" label="Edit" />
          <q-btn
            v-if="!detailNode.local"
            class="text-error-brighter"
            icon="delete"
            dense
            flat
            label="Remove"
            @click="deleteNodeAction(detailNode)" />
        </div>
      </div>

      <node-detail-panel
        v-if="detailNode"
        :key="detailNode.id"
        :node="detailNode"
        :snapshot="getSnapshot(detailNode.id)"
        :system-info="getNodeSummary(detailNode.id)?.systemInfo" />
      <empty-state
        v-else-if="!loading"
        description="It may have been removed, or the link is out of date."
        icon="dns"
        title="This node is no longer listed">
        <template #actions>
          <q-btn color="primary" label="Back to nodes" to="/nodes" />
        </template>
      </empty-state>
      <alert-rule-dialog
        v-if="detailNode"
        v-model="showAlertDialog"
        :node-id="detailNode.id"
        :node-name="detailNode.name || 'Unnamed node'" />
    </div>

    <q-dialog v-model="showDeleteDialog" aria-labelledby="node-remove-dialog-title" persistent>
      <q-card class="node-remove-dialog">
        <q-card-section>
          <div id="node-remove-dialog-title" class="text-h6 text-negative">Remove Node</div>
        </q-card-section>
        <q-card-section>
          <p class="q-mb-none">
            Are you sure you want to remove the node
            <strong>{{ selectedNodeForDelete?.name || selectedNodeForDelete?.baseUrl }}</strong
            >? This will also remove all cached remote server data from this node.
          </p>
          <q-banner
            v-if="selectedNodeServerCount > 0"
            class="xy-banner-negative q-mt-md"
            dense
            role="alert">
            {{ serverCountLabel(selectedNodeServerCount) }} still on this node. Move or delete
            {{ selectedNodeServerCount === 1 ? 'it' : 'them' }} before removing the node.
          </q-banner>
          <q-banner v-if="removeError" class="xy-banner-negative q-mt-md" dense role="alert">
            {{ removeError }}
          </q-banner>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn :disable="removing" flat label="Cancel" @click="showDeleteDialog = false" />
          <q-btn
            :disable="selectedNodeServerCount > 0"
            :loading="removing"
            color="negative"
            label="Remove"
            unelevated
            @click="confirmDelete" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { usePersistedRef } from '@/utils/persisted-ref'
import { Notify, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  bytesToSize,
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { Node, NodeResourceSnapshot } from '@/proto/shared_pb'
import { AllNodeMetrics } from '@/proto/websocket_pb'
import {
  DashboardNodeSummary,
  ListNodesRequestSchema,
  RemoveNodeRequestSchema,
} from '@/proto/xylona_pb'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import AlertRuleDialog from '@/components/alerts/AlertRuleDialog.vue'
import NodeDetailPanel from '@/components/nodes/NodeDetailPanel.vue'
import NodeMetricUnavailable from '@/components/nodes/NodeMetricUnavailable.vue'
import {
  nodeHealthBadge,
  nodeLastSeenMs,
  nodeResourceHealth,
  nodeResourcePercent,
  splitNodeVersion,
  type NodeResource,
} from '@/components/nodes/node-display'
import { formatMetricAge } from '@/pages/game_servers/metrics-format'
import { formatTimestamp } from '@/utils/format-timestamp'
import {
  websocketConnectingQuietly,
  websocketStateAuthoritative,
} from '@/utils/websocket-connection'

const $q = useQuasar()
const route = useRoute()
const router = useRouter()
const rows = ref([] as Node[])
const loading: Ref<boolean> = ref(false)
const loadError = ref('')
const metricsLoading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const showAlertDialog = ref(false)
const selectedNodeForDelete = ref<Node | null>(null)
const removing = ref(false)
const removeError = ref('')
// Known once metrics load; the server refuses the removal either way.
const selectedNodeServerCount = computed(() =>
  selectedNodeForDelete.value
    ? (getSnapshot(selectedNodeForDelete.value.id)?.gameServerCount ?? 0)
    : 0,
)
const nowMs = ref(Date.now())
let clockTimer: ReturnType<typeof setInterval> | null = null
const dashboardSummaries = ref<DashboardNodeSummary[]>([])
const liveSnapshots = ref<Map<string, NodeResourceSnapshot>>(new Map())
const dashboardSnapshotsFresh = ref(false)
let fetchSequence = 0
let reconnectRefreshQueued = false
let nodeListUnmounted = false
let snapshotAuthorityGeneration = 0

const initialPagination = usePersistedRef('node-pagination', {
  rowsPerPage: 25,
  page: 1,
})

const totalServers = computed(() =>
  rows.value.reduce((sum, n) => sum + (getSnapshot(n.id)?.gameServerCount ?? 0), 0),
)
const runningServers = computed(() =>
  rows.value.reduce((sum, n) => sum + (getSnapshot(n.id)?.runningGameServerCount ?? 0), 0),
)
// Every snapshot reports the controller-wide user count, so take it once, not per node.
const totalUsers = computed(() =>
  rows.value.reduce((max, n) => Math.max(max, getSnapshot(n.id)?.userCount ?? 0), 0),
)

const detailNodeId = computed(() => {
  const id = route.params.id
  return typeof id === 'string' && id !== '' ? id : ''
})
const detailNode = computed(() => rows.value.find((node) => node.id === detailNodeId.value) ?? null)

function getNodeSummary(nodeId: string): DashboardNodeSummary | undefined {
  return dashboardSummaries.value.find((s) => s.node?.id === nodeId)
}

function getSnapshot(nodeId: string): NodeResourceSnapshot | undefined {
  const liveSnapshot = liveSnapshots.value.get(nodeId)
  if (liveSnapshot) {
    return liveSnapshot
  }
  if (!dashboardSnapshotsFresh.value) {
    return undefined
  }
  return getNodeSummary(nodeId)?.snapshot
}

function getNodeVersion(nodeId: string): string | undefined {
  return getNodeSummary(nodeId)?.systemInfo?.xylonaVersion
}

// Undefined before any snapshot arrives; null when the node couldn't take the reading.
function resourcePercent(nodeId: string, resource: NodeResource): number | null | undefined {
  const snapshot = getSnapshot(nodeId)
  return snapshot ? nodeResourcePercent(snapshot, resource) : undefined
}

const mobileMetrics: { resource: NodeResource; label: string }[] = [
  { resource: 'cpu', label: 'CPU' },
  { resource: 'memory', label: 'Memory' },
  { resource: 'disk', label: 'Disk' },
]

function metricClass(resource: NodeResource, percent: number | null | undefined): string {
  const level = nodeResourceHealth(resource, percent).level
  if (level === 'danger') return 'text-negative'
  if (level === 'warn') return 'text-warning'
  return ''
}

function shouldShowMetricSkeleton(nodeId: string): boolean {
  return metricsLoading.value && getSnapshot(nodeId) === undefined
}

function shouldShowVersionSkeleton(nodeId: string): boolean {
  return metricsLoading.value && !getNodeVersion(nodeId)
}

function onNodeMetrics(metrics: AllNodeMetrics | undefined) {
  if (!websocketStateAuthoritative.value || !metrics?.nodes) return

  const nextSnapshots = new Map(liveSnapshots.value)
  for (const [nodeId, snapshot] of Object.entries(metrics.nodes)) {
    nextSnapshots.set(nodeId, snapshot)
  }
  liveSnapshots.value = nextSnapshots
}

function handleWebsocketDisconnect() {
  snapshotAuthorityGeneration++
  reconnectRefreshQueued = false
  dashboardSnapshotsFresh.value = false
  liveSnapshots.value = new Map()
  metricsLoading.value = false
}

function handleWebsocketReconnect() {
  reconnectRefreshQueued = true
  runQueuedReconnectRefresh()
}

function runQueuedReconnectRefresh() {
  if (nodeListUnmounted || !reconnectRefreshQueued || !websocketStateAuthoritative.value) {
    return
  }

  reconnectRefreshQueued = false
  void fetchAll()
}

onMounted(async () => {
  GetOrCreateXylonaWebsocketClient()
  XylonaEventBus.on('nodeMetrics', onNodeMetrics)
  XylonaEventBus.on('websocketConnected', handleWebsocketReconnect)
  XylonaEventBus.on('websocketDisconnected', handleWebsocketDisconnect)
  clockTimer = setInterval(() => (nowMs.value = Date.now()), 10_000)
  await fetchAll()
})

onBeforeUnmount(() => {
  nodeListUnmounted = true
  reconnectRefreshQueued = false
  XylonaEventBus.off('nodeMetrics', onNodeMetrics)
  XylonaEventBus.off('websocketConnected', handleWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', handleWebsocketDisconnect)
  if (clockTimer) clearInterval(clockTimer)
})

async function fetchAll() {
  const fetchID = ++fetchSequence
  const snapshotGeneration = snapshotAuthorityGeneration
  loading.value = true
  loadError.value = ''
  if (!dashboardSnapshotsFresh.value && liveSnapshots.value.size === 0) {
    metricsLoading.value = true
  }
  const dashboardPromise = GetXylonaClient()
    .getDashboardOverview({})
    .then((dashResp) => {
      if (fetchID !== fetchSequence) {
        return
      }
      dashboardSummaries.value = dashResp.nodes
      dashboardSnapshotsFresh.value =
        snapshotGeneration === snapshotAuthorityGeneration && websocketStateAuthoritative.value
    })
    .catch(() => null)
    .finally(() => {
      if (fetchID === fetchSequence) {
        metricsLoading.value = false
      }
    })

  try {
    const nodesResp = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    if (fetchID !== fetchSequence) {
      return
    }
    rows.value = nodesResp.nodes ? [...nodesResp.nodes] : []
  } catch (unknownError: unknown) {
    if (fetchID !== fetchSequence) {
      return
    }
    const err = ConnectError.from(unknownError)
    loadError.value = ConnectErrorToString(err)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
  } finally {
    if (fetchID === fetchSequence) {
      loading.value = false
    }
  }

  await dashboardPromise
  if (fetchID === fetchSequence) {
    runQueuedReconnectRefresh()
  }
}

// The glyph keeps a high or critical reading distinguishable without its colour.
function formatMetric(value: number | null | undefined, resource: NodeResource): string {
  if (value === undefined || value === null) return '—'
  const glyph = nodeResourceHealth(resource, value).glyph
  return `${glyph ? `${glyph} ` : ''}${Math.round(value)}%`
}

function lastSeenRelative(node: Node): string {
  const seenMs = nodeLastSeenMs(node)
  return seenMs === null ? 'never' : formatMetricAge(seenMs, nowMs.value)
}

function lastSeenAbsolute(node: Node): string {
  const seenMs = nodeLastSeenMs(node)
  return seenMs === null ? 'Never seen' : formatTimestamp(new Date(seenMs))
}

function openDetail(node: Node) {
  void router.push(`/nodes/${node.id}`)
}

function deleteNodeAction(node: Node) {
  selectedNodeForDelete.value = node
  removeError.value = ''
  showDeleteDialog.value = true
}

function serverCountLabel(count: number): string {
  return count === 1 ? '1 game server is' : `${count} game servers are`
}

async function confirmDelete() {
  if (!selectedNodeForDelete.value || removing.value) return
  removing.value = true
  removeError.value = ''
  try {
    await GetXylonaClient().removeNode(
      create(RemoveNodeRequestSchema, { nodeId: selectedNodeForDelete.value.id }),
    )
    showDeleteDialog.value = false
    if (detailNode.value?.id === selectedNodeForDelete.value.id) {
      await router.replace('/nodes')
    }
    selectedNodeForDelete.value = null
    await fetchAll()
  } catch (unknownError: unknown) {
    removeError.value = ConnectErrorToString(ConnectError.from(unknownError))
  } finally {
    removing.value = false
  }
}

const columns = ref([
  {
    name: 'name',
    label: 'Name',
    align: 'left' as const,
    field: (row: Node) => row.name,
    sortable: true,
  },
  {
    name: 'health',
    label: 'Status',
    align: 'left' as const,
    field: () => '',
    sortable: false,
  },
  {
    name: 'cpu',
    label: 'CPU',
    align: 'left' as const,
    field: (row: Node) => resourcePercent(row.id, 'cpu') ?? -1,
    sortable: true,
  },
  {
    name: 'ram',
    label: 'RAM',
    align: 'left' as const,
    field: (row: Node) => resourcePercent(row.id, 'memory') ?? -1,
    sortable: true,
  },
  {
    name: 'disk',
    label: 'Disk',
    align: 'left' as const,
    field: (row: Node) => resourcePercent(row.id, 'disk') ?? -1,
    sortable: true,
  },
  {
    name: 'servers',
    label: 'Servers',
    align: 'left' as const,
    field: (row: Node) => getSnapshot(row.id)?.gameServerCount ?? -1,
    sortable: true,
  },
  {
    name: 'version',
    label: 'Version',
    align: 'left' as const,
    field: () => '',
    sortable: false,
  },
  {
    name: 'lastSync',
    label: 'Last Seen',
    align: 'left' as const,
    field: (row: Node) => row.lastSeenAt,
    sortable: false,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
])
</script>

<style scoped>
.badge-remote {
  background-color: var(--xy-purple);
  color: var(--xy-text-primary);
}
.badge-auto {
  background-color: var(--xy-accent);
  color: var(--xy-base);
}

.node-remove-dialog {
  width: min(30rem, 100%);
}

.node-list__metric-skeleton {
  opacity: 0.78;
}

.node-list__metric-skeleton :deep(.q-skeleton) {
  background: color-mix(in srgb, var(--xy-text-muted) 18%, transparent);
}

.node-version {
  font-family: var(--xy-font-mono);
  white-space: nowrap;
}

.node-version small {
  margin-left: var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.list-notice {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  background: var(--xy-warning-bg-faint);
  border: 1px solid var(--xy-warning-border-soft);
  border-radius: var(--xy-radius-md);
}

.list-error {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
}

.list-error > div {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.list-error span {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.node-grid-item {
  padding: var(--xy-space-xs);
}

.node-mobile-card {
  height: 100%;
  overflow: hidden;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.node-mobile-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
}

.node-mobile-name {
  flex: 1;
  overflow: hidden;
  padding: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: none;
  border: 0;
  cursor: pointer;
}

.node-mobile-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.node-mobile-metrics > div {
  display: grid;
  gap: var(--xy-space-2xs);
  text-align: center;
}

.node-mobile-metrics span,
.node-mobile-meta {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.node-mobile-metrics strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
}

.node-mobile-meta {
  display: flex;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: 0 var(--xy-space-md) var(--xy-space-md);
}

.node-mobile-meta span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-mobile-actions {
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

@media (max-width: 599px) {
  .node-grid-item {
    padding-inline: 0;
  }

  .node-mobile-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .node-mobile-meta {
    display: grid;
  }
}
</style>
