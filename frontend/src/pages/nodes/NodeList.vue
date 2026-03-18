<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="xy-page-title">Nodes</div>
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
            <router-link class="table-link" :to="'/nodes/' + props.row.id + '/edit'">
              {{ props.row.name || 'Unnamed' }}
            </router-link>
            <q-badge v-if="props.row.local" color="primary" class="q-ml-sm" label="local" />
            <q-badge v-else color="purple" class="q-ml-sm" label="remote" />
          </q-td>
        </template>
        <template v-slot:body-cell-health="props">
          <q-td :props="props">
            <template v-if="!props.row.local">
              <q-badge
                :color="healthColor(props.row.healthStatus)"
                :label="props.row.healthStatus || 'unknown'" />
            </template>
            <span v-else class="text-grey">—</span>
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
            <span v-else class="text-grey">—</span>
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
    <q-dialog v-model="showDeleteDialog" aria-labelledby="dialog-title">
      <q-card>
        <q-card-section>
          <div id="dialog-title" class="text-h6">Remove Node</div>
        </q-card-section>
        <q-card-section>
          Are you sure you want to remove the node
          <strong>{{ selectedNode?.name || selectedNode?.baseUrl }}</strong
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
import { onMounted, Ref, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Node } from '@/proto/shared_pb'
import {
  ListNodesRequestSchema,
  RemoveNodeRequestSchema,
  SyncNodeRequestSchema,
} from '@/proto/xylona_pb'

const $q = useQuasar()
const rows = ref([] as Node[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const selectedNode = ref<Node | null>(null)

const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1,
})

onMounted(async () => {
  await getNodes()
})

async function getNodes() {
  loading.value = true
  try {
    const response = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    rows.value = response.nodes ? [...response.nodes] : []
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
  switch (status) {
    case 'healthy':
      return 'positive'
    case 'offline':
      return 'negative'
    default:
      return 'grey'
  }
}

function formatTimestamp(ts: { seconds: bigint }): string {
  if (!ts || !ts.seconds) return 'Never'
  const date = new Date(Number(ts.seconds) * 1000)
  return date.toLocaleString()
}

async function syncNode(node: Node) {
  try {
    await GetXylonaClient().syncNode(create(SyncNodeRequestSchema, { nodeId: node.id }))
    setTimeout(() => getNodes(), 2000)
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
  selectedNode.value = node
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!selectedNode.value) return
  try {
    await GetXylonaClient().removeNode(
      create(RemoveNodeRequestSchema, { nodeId: selectedNode.value.id }),
    )
    showDeleteDialog.value = false
    selectedNode.value = null
    await getNodes()
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
    label: 'Health',
    align: 'left' as const,
    field: (row: Node) => row.healthStatus,
    sortable: true,
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

<style scoped></style>
