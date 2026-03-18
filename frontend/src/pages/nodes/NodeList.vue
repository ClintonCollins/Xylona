<template>
  <q-page class="xy-page-content">
    <div v-if="!detailNode">
      <div class="xy-page-header">
        <div>
          <div class="xy-page-title">Nodes</div>
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
            dense
            outlined
            debounce="300"
            color="primary"
            v-model="search"
            placeholder="Search..."
            aria-label="Search nodes"
            style="min-width: 200px">
            <template v-slot:append>
              <q-icon name="search" />
            </template>
          </q-input>
          <q-btn color="primary" to="/nodes/add" label="Add Node" />
        </div>
      </div>
      <div>
        <q-table
          flat
          class="xy-standalone-table"
          :grid="$q.screen.lt.md"
          :rows="rows"
          :columns="columns"
          row-key="id"
          :filter="search"
          :loading="loading"
          v-model:pagination="initialPagination"
          hide-header-in-grid>
          <template v-slot:body-cell-name="props">
            <q-td :props="props">
              <a class="table-link" href="#" @click.prevent="openDetail(props.row)">
                {{ props.row.name || 'Unnamed' }}
              </a>
              <q-badge v-if="props.row.local" color="primary" class="q-ml-sm" label="local" />
              <q-badge v-else color="purple" class="q-ml-sm" label="remote" />
            </q-td>
          </template>
          <template v-slot:body-cell-health="props">
            <q-td :props="props">
              <template v-if="!props.row.local">
                <q-badge
                  :color="healthColor(props.row.healthStatus)"
                  :label="healthLabel(props.row.healthStatus)" />
              </template>
              <span v-else class="text-grey">&mdash;</span>
            </q-td>
          </template>
          <template v-slot:body-cell-version="props">
            <q-td :props="props">
              <span v-if="getNodeVersion(props.row.id)">
                v{{ getNodeVersion(props.row.id) }}
              </span>
              <span v-else class="text-grey">&mdash;</span>
            </q-td>
          </template>
          <template v-slot:body-cell-lastSync="props">
            <q-td :props="props">
              <template v-if="!props.row.local">
                <span v-if="props.row.lastSyncAt?.seconds">
                  {{ formatTimestamp(props.row.lastSyncAt) }}
                </span>
                <span v-else class="text-grey">Never</span>
                <q-badge
                  v-if="props.row.lastSyncStatus === 'error'"
                  color="negative"
                  class="q-ml-sm"
                  label="error" />
                <q-badge
                  v-else-if="props.row.lastSyncStatus === 'success'"
                  color="positive"
                  class="q-ml-sm"
                  label="ok" />
              </template>
              <span v-else class="text-grey">&mdash;</span>
            </q-td>
          </template>
          <template v-slot:body-cell-dateAdded="props">
            <q-td :props="props">
              <span v-if="props.row.createdAt?.seconds">
                {{ formatTimestamp(props.row.createdAt) }}
              </span>
              <span v-else class="text-grey">&mdash;</span>
            </q-td>
          </template>
          <template v-slot:body-cell-actions="props">
            <q-td :props="props">
              <div class="q-gutter-xs" v-if="!props.row.local">
                <q-btn
                  flat
                  dense
                  color="primary"
                  icon="sync"
                  aria-label="Sync node"
                  @click="syncNode(props.row)">
                  <q-tooltip>Sync now</q-tooltip>
                </q-btn>
                <router-link :to="'/nodes/' + props.row.id + '/edit'">
                  <q-btn
                    flat
                    dense
                    class="text-main-brighter"
                    :icon="tabSettings"
                    aria-label="Edit node">
                    <q-tooltip>Edit node</q-tooltip>
                  </q-btn>
                </router-link>
                <q-btn
                  flat
                  dense
                  class="text-error-brighter"
                  :icon="tabTrash"
                  aria-label="Delete node"
                  @click="deleteNodeAction(props.row)">
                  <q-tooltip>Remove node</q-tooltip>
                </q-btn>
              </div>
              <div class="q-gutter-xs" v-else>
                <router-link :to="'/nodes/' + props.row.id + '/edit'">
                  <q-btn
                    flat
                    dense
                    class="text-main-brighter"
                    :icon="tabSettings"
                    aria-label="Edit node">
                    <q-tooltip>Edit node</q-tooltip>
                  </q-btn>
                </router-link>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-xy-secondary">
              <q-icon name="dns" size="3rem" class="q-mb-sm text-xy-muted" />
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
            flat
            dense
            round
            icon="arrow_back"
            aria-label="Back to nodes"
            @click="detailNode = null" />
          <div class="text-h6 q-ml-sm">{{ detailNode.name || 'Node Details' }}</div>
          <q-badge
            v-if="detailNode.local"
            color="primary"
            class="q-ml-sm"
            label="local" />
          <q-badge v-else color="purple" class="q-ml-sm" label="remote" />
        </div>
        <div class="xy-page-actions" v-if="!detailNode.local">
          <q-btn
            flat
            dense
            color="primary"
            icon="sync"
            label="Sync"
            @click="syncNode(detailNode)" />
          <q-btn
            flat
            dense
            class="text-main-brighter"
            :icon="tabSettings"
            label="Edit"
            :to="'/nodes/' + detailNode.id + '/edit'" />
          <q-btn
            flat
            dense
            class="text-error-brighter"
            :icon="tabTrash"
            label="Remove"
            @click="deleteNodeAction(detailNode)" />
        </div>
        <div class="xy-page-actions" v-else>
          <q-btn
            flat
            dense
            class="text-main-brighter"
            :icon="tabSettings"
            label="Edit"
            :to="'/nodes/' + detailNode.id + '/edit'" />
        </div>
      </div>

      <NodeDetailPanel
        :node="detailNode"
        :system-info="getNodeSummary(detailNode.id)?.systemInfo"
        :snapshot="getNodeSummary(detailNode.id)?.snapshot" />
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
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn flat label="Remove" color="negative" @click="confirmDelete" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useStorage } from '@vueuse/core'
import { Notify, useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, onMounted, Ref, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Node } from '@/proto/shared_pb'
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
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const selectedNodeForDelete = ref<Node | null>(null)
const detailNode = ref<Node | null>(null)
const dashboardSummaries = ref<DashboardNodeSummary[]>([])

const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1,
})

const totalServers = computed(() =>
  dashboardSummaries.value.reduce((sum, s) => sum + (s.snapshot?.gameServerCount ?? 0), 0),
)
const runningServers = computed(() =>
  dashboardSummaries.value.reduce(
    (sum, s) => sum + (s.snapshot?.runningGameServerCount ?? 0),
    0,
  ),
)
const totalUsers = computed(() =>
  dashboardSummaries.value.reduce((sum, s) => sum + (s.snapshot?.userCount ?? 0), 0),
)

function getNodeSummary(nodeId: string): DashboardNodeSummary | undefined {
  return dashboardSummaries.value.find((s) => s.node?.id === nodeId)
}

function getNodeVersion(nodeId: string): string | undefined {
  return getNodeSummary(nodeId)?.systemInfo?.xylonaVersion
}

onMounted(async () => {
  await fetchAll()
})

async function fetchAll() {
  loading.value = true
  try {
    const [nodesResp, dashResp] = await Promise.all([
      GetXylonaClient().listNodes(create(ListNodesRequestSchema, {})),
      GetXylonaClient().getDashboardOverview({}).catch(() => null),
    ])
    rows.value = nodesResp.nodes ? [...nodesResp.nodes] : []
    if (dashResp) {
      dashboardSummaries.value = dashResp.nodes
    }
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
  } finally {
    loading.value = false
  }
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
    name: 'version',
    label: 'Xylona Version',
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
  {
    name: 'dateAdded',
    label: 'Date Added',
    align: 'left' as const,
    field: (row: Node) => row.createdAt,
    sortable: true,
  },
  { name: 'actions', label: '', align: 'center' as const, field: () => '' },
])
</script>

<style scoped></style>
