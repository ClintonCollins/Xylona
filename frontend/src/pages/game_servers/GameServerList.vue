<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Game Servers"
                                :rows="displayRows"
                                :columns="columns"
                                row-key="compositeId"
                                selection="multiple"
                                :filter="search"
                                :loading="loading"
                                v-model:pagination="initialPagination"
                                v-model:selected="selectedGameServers">
                            <template v-slot:top>
                                <div class="row col flex justify-between flex-center">
                                    <div class="col-12 col-md-6">
                                        <span class="text-h6">Game Servers</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" to="/game-servers/create" :disable="loading"
                                                   label="Create Game Server"/>
                                            <q-btn v-if="selectedGameServers.length >= 1" class="q-ml-sm" color="red"
                                                   :disable="loading"
                                                   label="Remove game server" @click="deleteGameServerAction(null)"/>
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
                                    <router-link class="table-link" :to="'/game-servers/'+props.row.id+'/console'">
                                        {{ props.row.displayName }}
                                    </router-link>
                                    <q-badge v-if="!props.row.isLocal" color="blue-grey" class="q-ml-sm" label="remote"/>
                                    <q-badge v-if="props.row.isStale" color="orange" class="q-ml-xs" label="stale"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-status="props">
                                <q-td :props="props">
                                    <StatusBadge style="margin-left: -1em" :status="props.row.statusEnum"></StatusBadge>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-node="props">
                                <q-td :props="props">
                                    <span>{{ props.row.nodeName }}</span>
                                    <q-badge v-if="props.row.isLocal" color="green" class="q-ml-sm" label="local"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs" v-if="props.row.isLocal">
                                        <router-link :to="'/game-servers/' + props.row.id + '/configuration'">
                                            <q-btn flat class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit game server</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <span>
                                            <q-btn flat class="text-error-brighter"
                                                   :icon="tabTrash" @click="deleteGameServerAction(props.row)">
                                                <q-tooltip>Delete game server</q-tooltip>
                                            </q-btn>
                                        </span>
                                    </div>
                                    <div v-else class="q-gutter-xs">
                                        <router-link :to="'/game-servers/' + props.row.id + '/console'">
                                            <q-btn flat class="text-info" icon="terminal">
                                                <q-tooltip>View console</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                    </div>
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
            <DeleteGameServerDialog :game-servers="selectedLocalServersForDelete" v-model:showDialog="showDeleteGameServerDialog"
                                    @submit="deleteGameServerSubmitted"></DeleteGameServerDialog>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth, XylonaEventBus } from '@/utils/shared'
import DeleteGameServerDialog from '@/components/game_servers/DeleteGameServerDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { Node, Status } from '@/proto/shared_pb'
import { useStorage } from '@vueuse/core'
import { AggregatedGameServer, ListAggregatedGameServersRequestSchema, ListNodesRequestSchema } from '@/proto/xylona_pb'

interface DisplayRow {
  compositeId: string
  id: string
  isLocal: boolean
  displayName: string
  gameName: string
  userName: string
  statusEnum: Status
  nodeName: string
  isStale: boolean
  sourceNodeId: string
}

const aggregatedServers = ref([] as AggregatedGameServer[])
const nodesByID = ref(new Map<string, Node>())
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const selectedGameServers = ref([] as DisplayRow[])

const initialPagination = useStorage('game-server-pagination', {
    rowsPerPage: 25,
    page: 1
})

const windowWidth = WindowWidth()

const displayRows = computed((): DisplayRow[] => {
  return aggregatedServers.value.map((server) => {
    if (server.isLocal && server.localServer) {
      const ls = server.localServer
      const nodeName = nodesByID.value.get(ls.nodeId)?.name || ls.nodeName || ls.nodeHost || 'Local'
      return {
        compositeId: 'local/' + ls.id,
        id: ls.id,
        isLocal: true,
        displayName: ls.name,
        gameName: ls.gameName,
        userName: ls.userName,
        statusEnum: ls.status,
        nodeName,
        isStale: false,
        sourceNodeId: ''
      }
    }
    const rs = server.remoteServer!
    const nodeName = nodesByID.value.get(rs.sourceNodeId || rs.nodeId)?.name || rs.nodeName || rs.nodeHost || 'Remote'
    return {
      compositeId: rs.sourceNodeId + '/' + rs.remoteServerId,
      id: rs.remoteServerId,
      isLocal: false,
      displayName: rs.displayName,
      gameName: rs.gameName,
      userName: '',
      statusEnum: rs.status,
      nodeName,
      isStale: rs.isStale,
      sourceNodeId: rs.sourceNodeId
    }
  })
})

const selectedLocalServersForDelete = computed(() => {
  return selectedGameServers.value.filter(s => s.isLocal).map(s => ({ id: s.id, name: s.displayName }))
})

onMounted(async () => {
  await getGameServers()
  watchServerStatusChanges()
})

async function getGameServers() {
  loading.value = true
  try {
    const [serversResult, nodesResult] = await Promise.allSettled([
      GetXylonaClient().listAggregatedGameServers(
        create(ListAggregatedGameServersRequestSchema, {})
      ),
      GetXylonaClient().listNodes(
        create(ListNodesRequestSchema, {})
      )
    ])

    if (serversResult.status === 'fulfilled') {
      aggregatedServers.value = serversResult.value.servers
    } else {
      console.error(serversResult.reason)
    }

    if (nodesResult.status === 'fulfilled') {
      nodesByID.value = new Map(nodesResult.value.nodes.map(node => [node.id, node]))
    } else {
      console.error(nodesResult.reason)
      nodesByID.value = new Map()
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function watchServerStatusChanges() {
  XylonaEventBus.on('gameServerStatus', (serverID: string, serverStatus: Status) => {
    for (const server of aggregatedServers.value) {
      if (server.isLocal && server.localServer && server.localServer.id === serverID) {
        server.localServer.status = serverStatus
      }
      if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
        server.remoteServer.status = serverStatus
      }
    }
  })
}

async function deleteGameServerAction(row: DisplayRow | null) {
  if (row !== null) {
    selectedGameServers.value = [row]
  }
  showDeleteGameServerDialog.value = true
}

async function deleteGameServerSubmitted(error: unknown | boolean) {
  showDeleteGameServerDialog.value = false
  selectedGameServers.value = []
  if (!error) {
    void getGameServers()
  }
}

const columns = ref([
  {
    name: 'name',
    label: 'Name',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.displayName,
    sortable: true
  },
  {
    name: 'game',
    label: 'Game',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.gameName,
    sortable: true
  },
  {
    name: 'owner',
    label: 'Owner',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.userName,
    sortable: true
  },
  {
    name: 'status',
    label: 'Status',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.statusEnum,
    sortable: true
  },
  {
    name: 'node',
    label: 'Node',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.nodeName,
    sortable: true
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => ''
  }
])
</script>

<style scoped>
</style>
