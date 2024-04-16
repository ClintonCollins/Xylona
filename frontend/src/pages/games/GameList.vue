<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <q-card class="col">
        <q-card-section>
          <div class="q-pa-md">
            <q-table
              flat
              title="Games"
              :rows="rows"
              :columns="columns"
              row-key="name"
              selection="multiple"
              v-model:selected="selected">
            <template v-slot:body-cell-windows_support="props">
              <q-td :props="props">
                <q-icon name="check" size="md" v-if="props.row.windowsSupport" color="green" />
                <q-icon name="close" size="md" v-else color="red" />
              </q-td>
            </template>
            <template v-slot:body-cell-linux_support="props">
              <q-td :props="props">
                <q-icon name="check" size="md" v-if="props.row.linuxSupport" color="green" />
                <q-icon name="close" size="md" v-else color="red" />
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
import {onMounted, ref} from 'vue'
import {GetXylonaClient} from "src/utils/shared";
import {Game, ListGamesRequest} from "src/proto/xylona_pb";

const rows = ref([] as Game[])

onMounted(async () => {
  await getGames()
})

async function getGames() {
  const request = new ListGamesRequest()
  try {
    const response = await GetXylonaClient().listGames(request)
    response.games.forEach((game) => {
      rows.value.push(game)
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
    name: 'default_port',
    label: 'Default Port',
    align: 'left',
    field: (row: { defaultPort: any; }) => row.defaultPort,
    sortable: true
  },
  {
    name: 'default_query_port',
    label: 'Default Query Port',
    align: 'left',
    field: (row: { defaultQueryPort: any; }) => row.defaultQueryPort,
    sortable: true
  },
  {
    name: 'windows_support',
    label: 'Windows Support',
    align: 'left',
    field: (row: { windowsSupport: boolean; }) => row.windowsSupport,
    sortable: true
  },
  {
    name: 'linux_support',
    label: 'Linux Support',
    align: 'left',
    field: (row: { windowsSupport: boolean; }) => row.windowsSupport,
    sortable: true
  }
])

</script>

<style scoped>

</style>
