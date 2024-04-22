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
              v-model:selected="selected">
              <template v-slot:top>
                <div class="row col flex justify-between flex-center">
                  <div class="col-12 col-md-6">
                    <span class="text-h6">Game Servers</span>
                  </div>
                  <div class="col-12 col-md-6">
                    <div class="row flex q-gutter-xl justify-end">
                      <q-btn color="primary" to="/game-servers/create" :disable="loading" label="Create Game Server"/>
                      <q-btn v-if="selected.length == 1" class="q-ml-sm" color="red" :disable="loading"
                             label="Remove game server" @click="removeGameServer"/>
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
                  <router-link class="table-link" :to="'/game-servers/'+props.row.id+'/console'">{{ props.row.name}}</router-link>
                </q-td>
              </template>
            </q-table>
          </div>
        </q-card-section>
      </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import {onMounted, Ref, ref} from 'vue'
import {GetXylonaClient, WindowWidth} from "src/utils/shared";
import {
  CreateGameServerRequest,
  GameServer,
  ListGameServersRequest,
  RemoveGameServerRequest
} from "src/proto/xylona_pb";

const rows = ref([] as GameServer[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')

const windowWidth = WindowWidth()

onMounted(async () => {
  await getGameServers()
})

async function getGameServers() {
  const request = new ListGameServersRequest()
  try {
    const response = await GetXylonaClient().listGameServers(request)
    rows.value = []
    response.gameServers.forEach((gameServer) => {
      console.log(gameServer)
      rows.value.push(gameServer)
    })
    console.log(response)
  } catch (e) {
    console.error(e)
  }
}

async function removeGameServer() {
  const selectedGameServer = selected.value[0] as GameServer
  console.log(selectedGameServer.name)

  const request = new RemoveGameServerRequest()
  request.serverId = selectedGameServer.id
  try {
    const response = await GetXylonaClient().removeGameServer(request)
    console.log(response)
  } catch (e) {
    console.error(e)
  } finally {
    await getGameServers()
  }
}

const selected = ref([])
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
  },  {
    name: 'owner',
    label: 'Owner',
    required: true,
    align: 'left',
    field: (row: { userName: any; }) => row.userName,
    sortable: true
  },  {
    name: 'status',
    label: 'Status',
    required: true,
    align: 'left',
    field: (row: { status: any; }) => row.status,
    sortable: true
  },
])

</script>

<style scoped>

</style>
