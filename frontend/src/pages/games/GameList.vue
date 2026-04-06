<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Games</h1>
      <div class="xy-page-actions">
        <q-input
          v-model="search"
          dense
          outlined
          debounce="300"
          color="primary"
          placeholder="Search..."
          aria-label="Search games"
          class="xy-search-input">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn color="primary" to="/games/new" label="Add Game" />
      </div>
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selected"
        flat
        class="xy-standalone-table"
        :grid="$q.screen.lt.md"
        :rows="rows"
        :columns="columns"
        row-key="name"
        selection="multiple"
        :filter="search"
        hide-header-in-grid>
        <template #body-cell-name="props">
          <q-td :props="props">
            <router-link class="table-link" :to="'/games/' + props.row.id + '/edit'"
              >{{ props.row.name }}
            </router-link>
          </q-td>
        </template>
        <template #body-cell-windows_support="props">
          <q-td :props="props">
            <q-icon v-if="props.row.windowsSupport" name="check" size="md" color="positive" />
            <q-icon v-else name="close" size="md" color="negative" />
          </q-td>
        </template>
        <template #body-cell-linux_support="props">
          <q-td :props="props">
            <q-icon v-if="props.row.linuxSupport" name="check" size="md" color="positive" />
            <q-icon v-else name="close" size="md" color="negative" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="q-gutter-xs">
              <router-link :to="'/games/' + props.row.id + '/edit'">
                <q-btn flat class="text-main-brighter" :icon="tabSettings" aria-label="Edit game">
                  <q-tooltip>Edit game</q-tooltip>
                </q-btn>
              </router-link>
              <router-link :to="'/games/' + props.row.id + '/copy'">
                <q-btn flat class="text-success-brighter" :icon="tabCopy" aria-label="Copy game">
                  <q-tooltip>Copy game</q-tooltip>
                </q-btn>
              </router-link>
              <span>
                <q-btn
                  flat
                  class="text-error-brighter"
                  :icon="tabTrash"
                  aria-label="Delete game"
                  @click="deleteGameAction(props.row)">
                  <q-tooltip>Delete game</q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon name="sports_esports" size="3rem" class="q-mb-sm text-xy-muted" />
            <div class="text-subtitle1">No games found</div>
            <div class="text-caption text-xy-muted">Add a game to get started.</div>
          </div>
        </template>
      </q-table>
    </div>
    <game-delete-dialog
      v-model:show-dialog="showGameDeleteDialog"
      :game="selectedActionGame"
      @submit="deleteGameSubmitted"></game-delete-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useStorage } from '@vueuse/core'
import GameDeleteDialog from '@/components/games/GameDeleteDialog.vue'
import { tabCopy, tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { onMounted, Ref, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Game } from '@/proto/shared_pb'
import { ListGamesRequest, ListGamesRequestSchema, ListGamesResponse } from '@/proto/xylona_pb'

const $q = useQuasar()
const rows = ref([] as Game[])
const search: Ref<string> = ref('')
const showGameDeleteDialog = ref(false)
const selectedActionGame = ref<Game | null>(null)

// Use VueUse to store the pagination state automatically.
const initialPagination = useStorage('game-pagination', {
  rowsPerPage: 25,
  page: 1,
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
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load games: ' + ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
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
    field: (row: { name: string }) => row.name,
    sortable: true,
  },
  {
    name: 'default_port',
    label: 'Default Port',
    align: 'left',
    field: (row: { defaultPort: number }) => row.defaultPort,
    sortable: true,
  },
  {
    name: 'default_query_port',
    label: 'Default Query Port',
    align: 'left',
    field: (row: { defaultQueryPort: number }) => row.defaultQueryPort,
    sortable: true,
  },
  {
    name: 'windows_support',
    label: 'Windows Support',
    align: 'left',
    field: (row: { windowsSupport: boolean }) => row.windowsSupport,
    sortable: true,
  },
  {
    name: 'linux_support',
    label: 'Linux Support',
    align: 'left',
    field: (row: { windowsSupport: boolean }) => row.windowsSupport,
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center',
    field: () => '',
  },
])
</script>

<style scoped></style>
