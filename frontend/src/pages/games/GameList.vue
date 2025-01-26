<template>
    <q-page :padding="windowWidth > 1024">
        <div class="row justify-center">
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
                                :filter="search"
                                v-model:pagination="initialPagination"
                                v-model:selected="selected">
                            <template v-slot:top>
                                <div class="row col flex justify-between flex-center">
                                    <div class="col-12 col-md-6">
                                        <span class="text-h6">Game Servers</span>
                                    </div>
                                    <div class="col-12 col-md-6">
                                        <div class="row flex q-gutter-xl justify-end">
                                            <q-btn color="primary" to="/games/create" label="Add Game"/>
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
                                    <router-link class="table-link" :to="'/games/'+props.row.id+'/edit'">{{ props.row.name }}
                                    </router-link>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-windows_support="props">
                                <q-td :props="props">
                                    <q-icon name="check" size="md" v-if="props.row.windowsSupport" color="green"/>
                                    <q-icon name="close" size="md" v-else color="red"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-linux_support="props">
                                <q-td :props="props">
                                    <q-icon name="check" size="md" v-if="props.row.linuxSupport" color="green"/>
                                    <q-icon name="close" size="md" v-else color="red"/>
                                </q-td>
                            </template>
                            <template v-slot:body-cell-actions="props">
                                <q-td :props="props">
                                    <div class="q-gutter-xs">
                                        <router-link :to="'/games/' + props.row.id + '/edit'">
                                            <q-btn flat class="text-main-brighter" :icon="tabSettings">
                                                <q-tooltip>Edit game</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <router-link :to="'/games/' + props.row.id + '/copy'">
                                            <q-btn flat class="text-success-brighter" :icon="tabCopy">
                                                <q-tooltip>Copy game</q-tooltip>
                                            </q-btn>
                                        </router-link>
                                        <span>
                                            <q-btn flat class="text-error-brighter"
                                                   :icon="tabTrash" @click="deleteGameAction(props.row)">
                                                <q-tooltip>Delete game</q-tooltip>
                                            </q-btn>
                                        </span>
                                    </div>
                                </q-td>
                            </template>
                        </q-table>
                    </div>
                </q-card-section>
            </q-card>
            <GameDeleteDialog :game="selectedActionGame" v-model:showDialog="showGameDeleteDialog" @submit="deleteGameSubmitted"></GameDeleteDialog>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useStorage } from '@vueuse/core'
import GameDeleteDialog from 'components/games/GameDeleteDialog.vue'
import {
  tabCopy,
  tabSettings,
  tabTrash,
} from 'quasar-extras-svg-icons/tabler-icons-v2'
import { onMounted, Ref, ref } from 'vue'
import { GetXylonaClient, WindowWidth } from 'src/utils/shared'
import { useRouter } from 'vue-router'
import { Game } from 'src/proto/shared_pb'
import { ListGamesRequest, ListGamesRequestSchema, ListGamesResponse
} from 'src/proto/xylona_pb'

const windowWidth = WindowWidth()
const rows = ref([] as Game[])
const search: Ref<string> = ref('')
const showGameDeleteDialog = ref(false)
const selectedActionGame = ref<Game | null>(null)

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('game-pagination', {
  rowsPerPage: 25,
  page: 1
})

onMounted(async () => {
  await getGames()
})

async function getGames() {
  const request: ListGamesRequest = create(ListGamesRequestSchema, {})
  try {
    const response: ListGamesResponse = await GetXylonaClient().listGames(request)
    rows.value = []
    response.games.forEach((game) => {
      rows.value.push(game)
    })
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    console.error(err.message)
  }
}

async function deleteGameAction(game: Game) {
  selectedActionGame.value = game
  showGameDeleteDialog.value = true
}

async function deleteGameSubmitted(error: unknown | boolean) {
  if (!error) {
    void getGames()
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
