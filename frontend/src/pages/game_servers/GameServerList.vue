<template>
  <q-page padding>
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
                      <q-btn color="primary" to="/game-servers/create" :disable="loading" label="Add game server"/>
                      <q-btn v-if="rows.length !== 0" class="q-ml-sm" color="primary" :disable="loading"
                             label="Remove game server"/>
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
                  <router-link :to="'/game-servers/'+props.row.id+'/console'">{{ props.row.name}}</router-link>
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
import {GetXylonaClient} from "src/utils/shared";
import {GameServer, ListGameServersRequest} from "src/proto/xylona_pb";

const rows = ref([] as GameServer[])
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')

onMounted(async () => {
  await getGameServers()
})

async function getGameServers() {
  const request = new ListGameServersRequest()
  try {
    const response = await GetXylonaClient().listGameServers(request)
    response.gameServers.forEach((gameServer) => {
      console.log(gameServer)
      rows.value.push(gameServer)
    })
    console.log(response)
  } catch (e) {
    console.error(e)
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
