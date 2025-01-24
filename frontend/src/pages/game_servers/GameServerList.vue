<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
            <q-card class="col">
                <q-card-section>
                    <div class="q-pa-md">
                        <q-table
                                flat
                                title="Game Servers"
                                :rows="rows"
                                :columns="columns"
                                row-key="name"
                                selection="multiple"
                                :filter="search"
                                :loading="loading"
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
import { tabCopy, tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth } from 'src/utils/shared'
import { ListGameServersRequest, ListGameServersRequestSchema } from 'src/proto/xylona_pb'
import DeleteGameServerDialog from '../../components/game_servers/DeleteGameServerDialog.vue'
import { Game, GameServer, RemoveGameServerRequest, RemoveGameServerRequestSchema } from '../../proto/shared_pb'

const rows = ref([] as GameServer[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const selectedGameServers = ref([])

const windowWidth = WindowWidth()

onMounted(async () => {
  await getGameServers()
})

async function getGameServers() {
  const request: ListGameServersRequest = create(ListGameServersRequestSchema, {})
  try {
    const response = await GetXylonaClient().listGameServers(request)
    rows.value = []
    response.gameServers.forEach((gameServer) => {
      // console.log(gameServer)
      rows.value.push(gameServer)
    })
    console.log(response)
  } catch (e) {
    console.error(e)
  }
}

async function deleteGameServerAction(gameServer: GameServer | null) {
  if (gameServer !== null) {
    selectedGameServers.value = [gameServer]
  }
  showDeleteGameServerDialog.value = true
}

async function deleteGameServerSubmitted(error: unknown | boolean) {
  showDeleteGameServerDialog.value = false
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
    name: 'actions',
    label: '',
    align: 'center',
    field: () => ''
  }
])

</script>

<style scoped>

</style>
