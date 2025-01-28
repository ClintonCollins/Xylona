<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Game Servers"
                                :rows="gameServers"
                                :columns="columns"
                                row-key="name"
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
                                        {{ props.row.name }}
                                    </router-link>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-status="props">
                                <q-td :props="props">
                                    <StatusBadge style="margin-left: -1em" :status="props.row.status"></StatusBadge>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs">
                                        <router-link :to="'/game-servers/' + props.row.id + '/edit'">
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
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
            <DeleteGameServerDialog :game-servers="selectedGameServers" v-model:showDialog="showDeleteGameServerDialog"
                                    @submit="deleteGameServerSubmitted"></DeleteGameServerDialog>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth, XylonaEventBus } from 'src/utils/shared'
import { ListGameServersRequest, ListGameServersRequestSchema } from 'src/proto/xylona_pb'
import DeleteGameServerDialog from 'src/components/game_servers/DeleteGameServerDialog.vue'
import StatusBadge from 'src/components/StatusBadge.vue'
import { GameServer, Node, Status } from 'src/proto/shared_pb'
import { useStorage } from '@vueuse/core'

const gameServers = ref([] as GameServer[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const selectedGameServers = ref([])

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('game-server-pagination', {
    rowsPerPage: 25,
    page: 1
})

const windowWidth = WindowWidth()

onMounted(async () => {
  await getGameServers()
  watchServerStatusChanges()
})

async function getGameServers() {
  const request: ListGameServersRequest = create(ListGameServersRequestSchema, {})
  try {
    const response = await GetXylonaClient().listGameServers(request)
    gameServers.value = []
    response.gameServers.forEach((gameServer) => {
      gameServers.value.push(gameServer)
    })
  } catch (e) {
    console.error(e)
  }
}

function watchServerStatusChanges() {
  XylonaEventBus.on('gameServerStatus', (serverID: string, serverStatus: Status) => {
    for (const gameServer of gameServers.value) {
      if (gameServer.id === serverID) {
        gameServer.status = serverStatus
      }
    }
  })
}

async function deleteGameServerAction(gameServer: GameServer | null) {
  if (gameServer !== null) {
    selectedGameServers.value = [gameServer]
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
    align: 'left',
    field: (row: { name: any; }) => row.name,
    sortable: true
  },
  {
    name: 'game',
    label: 'Game',
    required: true,
    align: 'left',
    field: (row: { gameName: any; }) => row.gameName,
    sortable: true
  }, {
    name: 'owner',
    label: 'Owner',
    required: true,
    align: 'left',
    field: (row: { userName: any; }) => row.userName,
    sortable: true
  }, {
    name: 'status',
    label: 'Status',
    required: true,
    align: 'left',
    field: (row: { status: any; }) => row.status,
    sortable: true
  },
  {
    name: 'node',
    label: 'Node',
    required: true,
    align: 'left',
    field: (row: { node: any; }) => row.nodeName,
    sortable: true
  },
  {
    name: 'actions',
    label: '',
    align: 'center',
    field: () => ''
  }
])

</script>

<style scoped>

</style>
