<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Nodes"
                                :rows="rows"
                                :columns="columns"
                                row-key="id"
                                :filter="search"
                                v-model:pagination="initialPagination">
                            <template v-slot:top>
                                <div class="row col flex justify-between flex-center">
                                    <div class="col-12 col-md-6">
                                        <span class="text-h6">Nodes</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" to="/nodes/add" label="Add Node"/>
                                            <q-input dense debounce="300" color="primary" v-model="search">
                                                <template v-slot:append>
                                                    <q-icon name="search"/>
                                                </template>
                                            </q-input>
                                        </div>
                                    </div>
                                </div>
                            </template>
                            <template v-slot:body-cell-name="props">
                                <q-td :props="props">
                                    <router-link class="table-link" :to="'/nodes/'+props.row.id+'/edit'">
                                        {{ props.row.name || 'Unnamed' }}
                                    </router-link>
                                    <q-badge v-if="props.row.local" color="blue" class="q-ml-sm" label="local"/>
                                    <q-badge v-else color="purple" class="q-ml-sm" label="remote"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-health="props">
                                <q-td :props="props">
                                    <template v-if="!props.row.local">
                                        <q-badge :color="healthColor(props.row.healthStatus)" :label="props.row.healthStatus || 'unknown'"/>
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
                                        <q-badge v-if="props.row.lastSyncStatus === 'error'" color="red" class="q-ml-sm" label="error"/>
                                        <q-badge v-else-if="props.row.lastSyncStatus === 'success'" color="green" class="q-ml-sm" label="ok"/>
                                    </template>
                                    <span v-else class="text-grey">—</span>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs" v-if="!props.row.local">
                                        <q-btn flat dense color="primary" icon="sync" @click="syncNode(props.row)">
                                            <q-tooltip>Sync now</q-tooltip>
                                        </q-btn>
                                        <router-link :to="'/nodes/' + props.row.id + '/edit'">
                                            <q-btn flat dense class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit node</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <q-btn flat dense class="text-error-brighter"
                                               :icon="tabTrash" @click="deleteNodeAction(props.row)">
                                            <q-tooltip>Remove node</q-tooltip>
                                        </q-btn>
                                    </div>
                                    <div class="q-gutter-xs" v-else>
                                        <router-link :to="'/nodes/' + props.row.id + '/edit'">
                                            <q-btn flat dense class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit node</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                    </div>
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
        </div>
        <q-dialog v-model="showDeleteDialog">
            <q-card>
                <q-card-section>
                    <div class="text-h6">Remove Node</div>
                </q-card-section>
                <q-card-section>
                    Are you sure you want to remove the node <strong>{{ selectedNode?.name || selectedNode?.baseUrl }}</strong>?
                    This will also remove all cached remote server data from this node.
                </q-card-section>
                <q-card-actions align="right">
                    <q-btn flat label="Cancel" v-close-popup/>
                    <q-btn flat label="Remove" color="red" @click="confirmDelete"/>
                </q-card-actions>
            </q-card>
        </q-dialog>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useStorage } from '@vueuse/core'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth } from '@/utils/shared'
import { Node } from '@/proto/shared_pb'
import { SyncNodeRequestSchema } from '@/proto/federation_pb'
import {
  ListNodesRequestSchema,
  RemoveNodeRequestSchema,
} from '@/proto/xylona_pb'

const windowWidth = WindowWidth()
const rows = ref([] as Node[])
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const selectedNode = ref<Node | null>(null)

const initialPagination = useStorage('node-pagination', {
  rowsPerPage: 25,
  page: 1
})

onMounted(async () => {
  await getNodes()
})

async function getNodes() {
  try {
    const response = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    rows.value = response.nodes ? [...response.nodes] : []
  } catch (e) {
    console.error(e)
  }
}

function healthColor(status: string): string {
  switch (status) {
    case 'healthy': return 'green'
    case 'offline': return 'red'
    default: return 'grey'
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
  } catch (e) {
    console.error(e)
  }
}

function deleteNodeAction(node: Node) {
  selectedNode.value = node
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!selectedNode.value) return
  try {
    await GetXylonaClient().removeNode(create(RemoveNodeRequestSchema, { nodeId: selectedNode.value.id }))
    showDeleteDialog.value = false
    selectedNode.value = null
    await getNodes()
  } catch (e) {
    console.error(e)
  }
}

const columns = ref([
  { name: 'name', label: 'Name', align: 'left' as const, field: (row: Node) => row.name, sortable: true },
  { name: 'health', label: 'Health', align: 'left' as const, field: (row: Node) => row.healthStatus, sortable: true },
  { name: 'lastSync', label: 'Last Sync', align: 'left' as const, field: (row: Node) => row.lastSyncAt, sortable: false },
  { name: 'actions', label: '', align: 'center' as const, field: () => '' }
])
</script>

<style scoped>
</style>
