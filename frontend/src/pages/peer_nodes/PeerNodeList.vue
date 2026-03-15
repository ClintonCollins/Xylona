<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Peer Nodes"
                                :rows="rows"
                                :columns="columns"
                                row-key="id"
                                :filter="search"
                                v-model:pagination="initialPagination">
                            <template v-slot:top>
                                <div class="row col flex justify-between flex-center">
                                    <div class="col-12 col-md-6">
                                        <span class="text-h6">Peer Nodes</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" to="/peer-nodes/add" label="Add Peer Node"/>
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
                                    <router-link class="table-link" :to="'/peer-nodes/'+props.row.id+'/edit'">
                                        {{ props.row.name || props.row.nodeId || 'Unnamed' }}
                                    </router-link>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-health="props">
                                <q-td :props="props">
                                    <q-badge :color="healthColor(props.row.healthStatus)" :label="props.row.healthStatus || 'unknown'"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-lastSync="props">
                                <q-td :props="props">
                                    <span v-if="props.row.lastSyncAt?.seconds">
                                        {{ formatTimestamp(props.row.lastSyncAt) }}
                                    </span>
                                    <span v-else class="text-grey">Never</span>
                                    <q-badge v-if="props.row.lastSyncStatus === 'error'" color="red" class="q-ml-sm" label="error"/>
                                    <q-badge v-else-if="props.row.lastSyncStatus === 'success'" color="green" class="q-ml-sm" label="ok"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-enabled="props">
                                <q-td :props="props">
                                    <q-icon name="check" size="sm" v-if="props.row.enabled" color="green"/>
                                    <q-icon name="close" size="sm" v-else color="red"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs">
                                        <q-btn flat dense color="primary" icon="sync" @click="syncPeer(props.row)">
                                            <q-tooltip>Sync now</q-tooltip>
                                        </q-btn>
                                        <router-link :to="'/peer-nodes/' + props.row.id + '/edit'">
                                            <q-btn flat dense class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit peer</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <q-btn flat dense class="text-error-brighter"
                                               :icon="tabTrash" @click="deletePeer(props.row)">
                                            <q-tooltip>Remove peer</q-tooltip>
                                        </q-btn>
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
                    <div class="text-h6">Remove Peer Node</div>
                </q-card-section>
                <q-card-section>
                    Are you sure you want to remove the peer node <strong>{{ selectedPeer?.name || selectedPeer?.baseUrl }}</strong>?
                    This will also remove all cached remote server data from this peer.
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
import { PeerNode } from '@/proto/federation_pb'
import {
  ListPeerNodesRequestSchema,
  RemovePeerNodeRequestSchema,
  SyncPeerNodeRequestSchema
} from '@/proto/federation_pb'

const windowWidth = WindowWidth()
const rows = ref([] as PeerNode[])
const search: Ref<string> = ref('')
const showDeleteDialog = ref(false)
const selectedPeer = ref<PeerNode | null>(null)

const initialPagination = useStorage('peer-node-pagination', {
  rowsPerPage: 25,
  page: 1
})

onMounted(async () => {
  await getPeerNodes()
})

async function getPeerNodes() {
  try {
    const response = await GetXylonaClient().listPeerNodes(create(ListPeerNodesRequestSchema, {}))
    rows.value = response.peerNodes
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

async function syncPeer(peer: PeerNode) {
  try {
    await GetXylonaClient().syncPeerNode(create(SyncPeerNodeRequestSchema, { peerNodeId: peer.id }))
    setTimeout(() => getPeerNodes(), 2000)
  } catch (e) {
    console.error(e)
  }
}

function deletePeer(peer: PeerNode) {
  selectedPeer.value = peer
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!selectedPeer.value) return
  try {
    await GetXylonaClient().removePeerNode(create(RemovePeerNodeRequestSchema, { peerNodeId: selectedPeer.value.id }))
    showDeleteDialog.value = false
    selectedPeer.value = null
    await getPeerNodes()
  } catch (e) {
    console.error(e)
  }
}

const columns = ref([
  { name: 'name', label: 'Name', align: 'left' as const, field: (row: PeerNode) => row.name, sortable: true },
  { name: 'baseUrl', label: 'URL', align: 'left' as const, field: (row: PeerNode) => row.baseUrl, sortable: true },
  { name: 'health', label: 'Health', align: 'left' as const, field: (row: PeerNode) => row.healthStatus, sortable: true },
  { name: 'lastSync', label: 'Last Sync', align: 'left' as const, field: (row: PeerNode) => row.lastSyncAt, sortable: false },
  { name: 'version', label: 'Version', align: 'left' as const, field: (row: PeerNode) => row.version, sortable: true },
  { name: 'enabled', label: 'Enabled', align: 'center' as const, field: (row: PeerNode) => row.enabled, sortable: true },
  { name: 'actions', label: '', align: 'center' as const, field: () => '' }
])
</script>

<style scoped>
</style>
