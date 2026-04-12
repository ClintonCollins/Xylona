<template>
  <q-page class="xy-page-content">
    <div v-if="!detailNode">
      <div class="xy-page-header">
        <div>
          <h1 class="xy-page-title">Nodes</h1>
          <div class="text-caption text-xy-secondary" style="margin-top: 2px">
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
        </div>
        <div class="xy-page-actions">
          <q-input
            v-model="search"
            aria-label="Search nodes"
            class="xy-search-input"
            color="primary"
            debounce="300"
            dense
            outlined
            placeholder="Search...">
            <template #append>
              <q-icon name="search" />
            </template>
          </q-input>
          <q-btn color="primary" flat icon="notifications" label="Activity" to="/nodes/activity" />
          <q-btn color="primary" label="Add Node" to="/nodes/add" />
        </div>
      </div>
      <div>
        <q-table
          v-model:pagination="initialPagination"
          :columns="columns"
          :filter="search"
          :grid="$q.screen.lt.md"
          :loading="loading"
          :rows="rows"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #body-cell-name="props">
            <q-td :props="props">
              <button class="table-link" type="button" @click="openDetail(props.row)">
                {{ props.row.name || 'Unnamed' }}
              </button>
              <q-badge v-if="props.row.local" class="q-ml-sm" color="primary" label="local" />
              <q-badge v-else class="badge-remote q-ml-sm" label="remote" />
              <q-badge v-if="props.row.autoPaired" class="badge-auto q-ml-xs" label="auto" />
            </q-td>
          </template>
          <template #body-cell-health="props">
            <q-td :props="props">
              <template v-if="!props.row.local">
                <q-badge
                  :color="healthColor(props.row.healthStatus)"
                  :label="healthLabel(props.row.healthStatus)" />
              </template>
              <q-badge v-else color="positive" label="Healthy" />
            </q-td>
          </template>
          <template #body-cell-cpu="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="3rem" />
              </template>
              <template v-else-if="getSnapshot(props.row.id)">
                <span :class="'text-' + metricColor(getSnapshot(props.row.id)!.cpuPercent)">
                  {{ Math.round(getSnapshot(props.row.id)!.cpuPercent) }}%
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
              <template v-else-if="getSnapshot(props.row.id)">
                <span :class="'text-' + metricColor(getSnapshot(props.row.id)!.memoryPercent)">
                  {{ Math.round(getSnapshot(props.row.id)!.memoryPercent) }}%
                </span>
                <span class="text-caption text-xy-muted q-ml-xs">
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
              <template v-else-if="getSnapshot(props.row.id)">
                <span :class="'text-' + metricColor(getSnapshot(props.row.id)!.diskPercent)">
                  {{ Math.round(getSnapshot(props.row.id)!.diskPercent) }}%
                </span>
                <span class="text-caption text-xy-muted q-ml-xs">
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
              <template v-else-if="getSnapshot(props.row.id)">
                <span class="text-success">{{
                  getSnapshot(props.row.id)!.runningGameServerCount
                }}</span>
                /
                {{ getSnapshot(props.row.id)!.gameServerCount }}
              </template>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-users="props">
            <q-td :props="props">
              <template v-if="shouldShowMetricSkeleton(props.row.id)">
                <q-skeleton class="node-list__metric-skeleton" type="text" width="2.5rem" />
              </template>
              <template v-else-if="getSnapshot(props.row.id)">
                {{ getSnapshot(props.row.id)!.userCount }}
              </template>
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
              <span v-else-if="getNodeVersion(props.row.id)">
                {{ getNodeVersion(props.row.id) }}
              </span>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-lastSync="props">
            <q-td :props="props">
              <template v-if="!props.row.local">
                <span v-if="props.row.lastSyncAt?.seconds">
                  {{ formatTimestamp(props.row.lastSyncAt) }}
                </span>
                <span v-else class="text-xy-muted">Never</span>
                <q-badge
                  v-if="props.row.lastSyncStatus === 'error'"
                  class="q-ml-sm"
                  color="negative"
                  label="error" />
                <q-badge
                  v-else-if="props.row.lastSyncStatus === 'success'"
                  class="q-ml-sm"
                  color="positive"
                  label="ok" />
              </template>
              <span v-else class="text-xy-muted">&mdash;</span>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div v-if="!props.row.local" class="q-gutter-xs">
                <q-btn
                  aria-label="Sync node"
                  color="primary"
                  dense
                  flat
                  icon="sync"
                  @click="syncNode(props.row)">
                  <q-tooltip>Sync now</q-tooltip>
                </q-btn>
                <router-link :to="'/nodes/' + props.row.id + '/edit'">
                  <q-btn
                    :icon="tabSettings"
                    aria-label="Edit node"
                    class="text-main-brighter"
                    dense
                    flat>
                    <q-tooltip>Edit node</q-tooltip>
                  </q-btn>
                </router-link>
                <q-btn
                  :icon="tabTrash"
                  aria-label="Delete node"
                  class="text-error-brighter"
                  dense
                  flat
                  @click="deleteNodeAction(props.row)">
                  <q-tooltip>Remove node</q-tooltip>
                </q-btn>
              </div>
              <div v-else class="q-gutter-xs">
                <router-link :to="'/nodes/' + props.row.id + '/edit'">
                  <q-btn
                    :icon="tabSettings"
                    aria-label="Edit node"
                    class="text-main-brighter"
                    dense
                    flat>
                    <q-tooltip>Edit node</q-tooltip>
                  </q-btn>
                </router-link>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-xy-secondary">
              <q-icon class="q-mb-sm text-xy-muted" name="dns" size="3rem" />
              <div class="text-subtitle1">No nodes found</div>
              <div class="text-caption text-xy-muted">
                Add a remote node to get started with federation.
              </div>
            </div>
          </template>
        </q-table>
      </div>
    </div>

    <div v-if="detailNode">
      <div class="xy-page-header">
        <div class="row items-center">
          <q-btn
            aria-label="Back to nodes"
            dense
            flat
            icon="arrow_back"
            round
            @click="detailNode = null" />
          <div class="text-h6 q-ml-sm">{{ detailNode.name || 'Node Details' }}</div>
          <q-badge v-if="detailNode.local" class="q-ml-sm" color="primary" label="local" />
          <q-badge v-else class="badge-remote q-ml-sm" label="remote" />
        </div>
        <div v-if="!detailNode.local" class="xy-page-actions">
          <q-btn
            color="primary"
            dense
            flat
            icon="sync"
            label="Sync"
            @click="syncNode(detailNode)" />
          <q-btn
            :icon="tabSettings"
            :to="'/nodes/' + detailNode.id + '/edit'"
            class="text-main-brighter"
            dense
            flat
            label="Edit" />
          <q-btn
            :icon="tabTrash"
            class="text-error-brighter"
            dense
            flat
            label="Remove"
            @click="deleteNodeAction(detailNode)" />
        </div>
        <div v-else class="xy-page-actions">
          <q-btn
            :icon="tabSettings"
            :to="'/nodes/' + detailNode.id + '/edit'"
            class="text-main-brighter"
            dense
            flat
            label="Edit" />
        </div>
      </div>

      <node-detail-panel
        :node="detailNode"
        :snapshot="getSnapshot(detailNode.id)"
        :system-info="getNodeSummary(detailNode.id)?.systemInfo" />
    </div>

    <q-dialog v-model="showDeleteDialog" aria-labelledby="dialog-title">
      <q-card>
        <q-card-section>
          <div id="dialog-title" class="text-h6">Remove Node</div>
        </q-card-section>
        <q-card-section>
          Are you sure you want to remove the node
          <strong>{{ selectedNodeForDelete?.name || selectedNodeForDelete?.baseUrl }}</strong
          >? This will also remove all cached remote server data from this node.
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat label="Cancel" />
          <q-btn color="negative" flat label="Remove" @click="confirmDelete" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useStorage } from '@vueuse/core'
import { Notify, useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
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
  SyncNodeRequestSchema,
} from '@/proto/xylona_pb'
import NodeDetailPanel from '@/components/nodes/NodeDetailPanel.vue'

const $q = useQuasar()
const rows = ref([] as Node[])
const loading: Ref<boolean> = ref(false)
const metricsLoading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const selectedNodeForDelete = ref<Node | null>(null)
const detailNode = ref<Node | null>(null)
const dashboardSummaries = ref<DashboardNodeSummary[]>([])
const liveSnapshots = ref<Map<string, NodeResourceSnapshot>>(new Map())
let fetchSequence = 0

const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1,
})

const totalServers = computed(() =>
  rows.value.reduce((sum, n) => sum + (getSnapshot(n.id)?.gameServerCount ?? 0), 0),
)
const runningServers = computed(() =>
  rows.value.reduce((sum, n) => sum + (getSnapshot(n.id)?.runningGameServerCount ?? 0), 0),
)
const totalUsers = computed(() =>
  rows.value.reduce((sum, n) => sum + (getSnapshot(n.id)?.userCount ?? 0), 0),
)

function getNodeSummary(nodeId: string): DashboardNodeSummary | undefined {
  return dashboardSummaries.value.find((s) => s.node?.id === nodeId)
}

function getSnapshot(nodeId: string): NodeResourceSnapshot | undefined {
  return liveSnapshots.value.get(nodeId) ?? getNodeSummary(nodeId)?.snapshot
}

function getNodeVersion(nodeId: string): string | undefined {
  return getNodeSummary(nodeId)?.systemInfo?.xylonaVersion
}

function metricColor(percent: number): string {
  if (percent >= 80) return 'negative'
  if (percent >= 50) return 'warning'
  return 'positive'
}

function shouldShowMetricSkeleton(nodeId: string): boolean {
  return metricsLoading.value && getSnapshot(nodeId) === undefined
}

function shouldShowVersionSkeleton(nodeId: string): boolean {
  return metricsLoading.value && !getNodeVersion(nodeId)
}

function onNodeMetrics(metrics: AllNodeMetrics | undefined) {
  if (!metrics?.nodes) return
  for (const [nodeId, snapshot] of Object.entries(metrics.nodes)) {
    liveSnapshots.value.set(nodeId, snapshot)
  }
}

onMounted(async () => {
  GetOrCreateXylonaWebsocketClient()
  XylonaEventBus.on('nodeMetrics', onNodeMetrics)
  await fetchAll()
})

onBeforeUnmount(() => {
  XylonaEventBus.off('nodeMetrics', onNodeMetrics)
})

async function fetchAll() {
  const fetchID = ++fetchSequence
  loading.value = true
  if (dashboardSummaries.value.length === 0 && liveSnapshots.value.size === 0) {
    metricsLoading.value = true
  }
  const dashboardPromise = GetXylonaClient()
    .getDashboardOverview({})
    .then((dashResp) => {
      if (fetchID !== fetchSequence) {
        return
      }
      dashboardSummaries.value = dashResp.nodes
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

  void dashboardPromise
}

function healthColor(status: string): string {
  if (status === 'healthy') return 'positive'
  if (status === 'degraded') return 'warning'
  if (status === 'unreachable' || status === 'offline') return 'negative'
  return 'grey'
}

function healthLabel(status: string): string {
  if (status === 'healthy') return 'Healthy'
  if (status === 'degraded') return 'Degraded'
  if (status === 'unreachable') return 'Unreachable'
  return status || 'Unknown'
}

function formatTimestamp(ts: { seconds: bigint }): string {
  if (!ts || !ts.seconds) return 'Never'
  const date = new Date(Number(ts.seconds) * 1000)
  return date.toLocaleString()
}

function openDetail(node: Node) {
  detailNode.value = node
}

async function syncNode(node: Node) {
  try {
    await GetXylonaClient().syncNode(create(SyncNodeRequestSchema, { nodeId: node.id }))
    setTimeout(() => fetchAll(), 2000)
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
  }
}

function deleteNodeAction(node: Node) {
  selectedNodeForDelete.value = node
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!selectedNodeForDelete.value) return
  try {
    await GetXylonaClient().removeNode(
      create(RemoveNodeRequestSchema, { nodeId: selectedNodeForDelete.value.id }),
    )
    showDeleteDialog.value = false
    if (detailNode.value?.id === selectedNodeForDelete.value.id) {
      detailNode.value = null
    }
    selectedNodeForDelete.value = null
    await fetchAll()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
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
    field: (row: Node) => row.healthStatus,
    sortable: true,
  },
  {
    name: 'cpu',
    label: 'CPU',
    align: 'left' as const,
    field: (row: Node) => getSnapshot(row.id)?.cpuPercent ?? -1,
    sortable: true,
  },
  {
    name: 'ram',
    label: 'RAM',
    align: 'left' as const,
    field: (row: Node) => getSnapshot(row.id)?.memoryPercent ?? -1,
    sortable: true,
  },
  {
    name: 'disk',
    label: 'Disk',
    align: 'left' as const,
    field: (row: Node) => getSnapshot(row.id)?.diskPercent ?? -1,
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
    name: 'users',
    label: 'Users',
    align: 'left' as const,
    field: (row: Node) => getSnapshot(row.id)?.userCount ?? -1,
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
    label: 'Last Sync',
    align: 'left' as const,
    field: (row: Node) => row.lastSyncAt,
    sortable: false,
  },
  { name: 'actions', label: '', align: 'center' as const, field: () => '' },
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

.node-list__metric-skeleton {
  opacity: 0.78;
}

.node-list__metric-skeleton :deep(.q-skeleton) {
  background: color-mix(in srgb, var(--xy-text-muted) 18%, transparent);
}
</style>
