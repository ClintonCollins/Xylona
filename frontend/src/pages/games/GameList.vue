<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Games</h1>
      <div class="xy-page-actions">
        <q-input
          v-model="search"
          aria-label="Search games"
          class="xy-search-input"
          color="primary"
          debounce="300"
          dense
          outlined
          placeholder="Search games">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn
          color="secondary"
          icon="upload_file"
          label="Import JSON"
          @click="showGameImportDialog = true" />
        <q-btn color="primary" label="Add game" to="/games/new" />
      </div>
    </div>
    <div v-if="loadError" class="list-error" role="alert" aria-live="assertive">
      <q-icon name="sync_problem" size="sm" />
      <div>
        <strong>Games could not be loaded.</strong>
        <span>{{ loadError }}</span>
      </div>
      <q-btn :loading="loading" dense flat icon="refresh" label="Retry" @click="getGames" />
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selected"
        :columns="columns"
        :filter="search"
        :grid="$q.screen.lt.md"
        :loading="loading"
        :rows="rows"
        class="xy-standalone-table"
        flat
        hide-header-in-grid
        row-key="id"
        selection="multiple">
        <template #item="props">
          <div class="game-grid-item col-12 col-sm-6">
            <q-card class="game-mobile-card" flat>
              <q-card-section class="game-mobile-header">
                <q-checkbox
                  v-model="props.selected"
                  :aria-label="`Select ${props.row.name}`"
                  dense />
                <div class="game-mobile-identity">
                  <router-link :to="`/games/${props.row.id}/edit`" class="game-mobile-name">
                    {{ props.row.name }}
                  </router-link>
                  <q-badge
                    :color="props.row.xylonaOfficial ? 'positive' : 'grey-8'"
                    :label="props.row.xylonaOfficial ? 'Official' : 'Custom'" />
                </div>
              </q-card-section>

              <q-card-section class="game-mobile-details">
                <div>
                  <span>Game port</span>
                  <strong>{{ props.row.defaultPort || 'Not set' }}</strong>
                </div>
                <div>
                  <span>Query port</span>
                  <strong>{{ props.row.defaultQueryPort || 'Not set' }}</strong>
                </div>
                <div>
                  <span>Windows</span>
                  <strong>{{ props.row.windowsSupport ? 'Supported' : 'Not supported' }}</strong>
                </div>
                <div>
                  <span>Linux</span>
                  <strong>{{ props.row.linuxSupport ? 'Supported' : 'Not supported' }}</strong>
                </div>
              </q-card-section>

              <q-card-actions class="game-mobile-actions">
                <q-btn
                  :to="`/games/${props.row.id}/edit`"
                  color="primary"
                  flat
                  icon="edit"
                  label="Edit"
                  no-caps />
                <q-space />
                <q-btn
                  :to="`/games/${props.row.id}/copy`"
                  :aria-label="`Copy ${props.row.name}`"
                  flat
                  icon="content_copy">
                  <q-tooltip>Copy game</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Export ${props.row.name} as JSON`"
                  flat
                  icon="file_download"
                  @click="exportGameAction(props.row)">
                  <q-tooltip>Export JSON</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Delete ${props.row.name}`"
                  class="text-error-brighter"
                  flat
                  icon="delete"
                  @click="deleteGameAction(props.row)">
                  <q-tooltip>Delete game</q-tooltip>
                </q-btn>
              </q-card-actions>
            </q-card>
          </div>
        </template>
        <template #body-cell-name="props">
          <q-td :props="props">
            <router-link :to="'/games/' + props.row.id + '/edit'" class="table-link"
              >{{ props.row.name }}
            </router-link>
          </q-td>
        </template>
        <template #body-cell-xylona_official="props">
          <q-td :props="props">
            <q-badge
              :color="props.row.xylonaOfficial ? 'positive' : 'grey-8'"
              :label="props.row.xylonaOfficial ? 'Official' : 'Custom'" />
          </q-td>
        </template>
        <template #body-cell-windows_support="props">
          <q-td :props="props">
            <q-icon v-if="props.row.windowsSupport" color="positive" name="check" size="md" />
            <q-icon v-else color="negative" name="close" size="md" />
          </q-td>
        </template>
        <template #body-cell-linux_support="props">
          <q-td :props="props">
            <q-icon v-if="props.row.linuxSupport" color="positive" name="check" size="md" />
            <q-icon v-else color="negative" name="close" size="md" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="q-gutter-xs">
              <router-link :to="'/games/' + props.row.id + '/edit'">
                <q-btn :icon="tabSettings" aria-label="Edit game" class="text-main-brighter" flat>
                  <q-tooltip>Edit game</q-tooltip>
                </q-btn>
              </router-link>
              <router-link :to="'/games/' + props.row.id + '/copy'">
                <q-btn :icon="tabCopy" aria-label="Copy game" class="text-success-brighter" flat>
                  <q-tooltip>Copy game</q-tooltip>
                </q-btn>
              </router-link>
              <span>
                <q-btn
                  aria-label="Export game JSON"
                  class="text-xy-accent"
                  flat
                  icon="file_download"
                  @click="exportGameAction(props.row)">
                  <q-tooltip>Export game JSON</q-tooltip>
                </q-btn>
              </span>
              <span>
                <q-btn
                  :icon="tabTrash"
                  aria-label="Delete game"
                  class="text-error-brighter"
                  flat
                  @click="deleteGameAction(props.row)">
                  <q-tooltip>Delete game</q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon class="q-mb-sm text-xy-muted" name="sports_esports" size="3rem" />
            <div class="text-subtitle1">{{ search ? 'No matching games' : 'No games yet' }}</div>
            <div class="text-caption text-xy-muted">
              {{ search ? 'Try a different search.' : 'Add a game to get started.' }}
            </div>
            <q-btn
              v-if="!search"
              class="q-mt-md"
              color="primary"
              label="Add game"
              to="/games/new" />
          </div>
        </template>
      </q-table>
    </div>
    <game-delete-dialog
      v-if="selectedActionGame"
      v-model:show-dialog="showGameDeleteDialog"
      :game="selectedActionGame"
      @submit="deleteGameSubmitted"></game-delete-dialog>
    <game-import-dialog
      v-model:show-dialog="showGameImportDialog"
      @imported="gameImported"></game-import-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useStorage } from '@vueuse/core'
import GameDeleteDialog from '@/components/games/GameDeleteDialog.vue'
import GameImportDialog from '@/components/games/GameImportDialog.vue'
import { exportGameDefinitionJSON } from '@/components/games/game-definition-json'
import { tabCopy, tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { onMounted, Ref, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Game } from '@/proto/shared_pb'
import { ListGamesRequest, ListGamesRequestSchema, ListGamesResponse } from '@/proto/xylona_pb'

const $q = useQuasar()
const router = useRouter()
const rows = ref([] as Game[])
const search: Ref<string> = ref('')
const loading = ref(false)
const loadError = ref('')
const showGameDeleteDialog = ref(false)
const showGameImportDialog = ref(false)
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
  loading.value = true
  loadError.value = ''
  try {
    const response: ListGamesResponse = await GetXylonaClient().listGames(request)
    rows.value = [...response.games]
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    console.error(err.message)
    loadError.value = ConnectErrorToString(ConnectError.from(unknownError))
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load games: ' + ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  } finally {
    loading.value = false
  }
}

async function deleteGameAction(game: Game) {
  selectedActionGame.value = game
  showGameDeleteDialog.value = true
}

async function exportGameAction(game: Game) {
  try {
    const fileName = await exportGameDefinitionJSON(game.id)
    $q.notify({
      type: 'xylona-success',
      position: 'top',
      caption: `Exported ${fileName}.`,
      icon: 'check_circle',
    })
  } catch (unknownError: unknown) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to export game: ' + ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  }
}

async function deleteGameSubmitted(error: unknown | boolean) {
  if (!error) {
    void getGames()
  }
}

async function gameImported(gameID: string) {
  await getGames()
  if (gameID !== '') {
    await router.push(`/games/${gameID}/edit`)
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
    name: 'xylona_official',
    label: 'Source',
    align: 'left',
    field: (row: { xylonaOfficial: boolean }) => (row.xylonaOfficial ? 'Official' : 'Custom'),
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

<style scoped>
.list-error {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
}

.list-error > div {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.list-error span {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.game-grid-item {
  padding: var(--xy-space-xs);
}

.game-mobile-card {
  height: 100%;
  overflow: hidden;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.game-mobile-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
}

.game-mobile-identity {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.game-mobile-name {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.game-mobile-details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.game-mobile-details > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.game-mobile-details span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.game-mobile-details strong {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
}

.game-mobile-actions {
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

@media (max-width: 599px) {
  .game-grid-item {
    padding-inline: 0;
  }
}
</style>
