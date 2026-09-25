<template>
  <q-page class="xy-page-content">
    <page-header title="Games">
      <template #actions>
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
        <q-btn flat icon="upload_file" label="Import JSON" @click="showGameImportDialog = true" />
        <q-btn color="primary" label="Add game" to="/games/new" />
      </template>
    </page-header>
    <q-banner v-if="loadError" class="xy-banner-negative q-mb-md" dense inline-actions role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      <strong>Games could not be loaded.</strong> {{ loadError }}
      <template #action>
        <q-btn
          :loading="loading"
          aria-label="Retry loading games"
          flat
          icon="refresh"
          label="Retry"
          no-caps
          @click="getGames" />
      </template>
    </q-banner>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        aria-label="Games"
        :columns="columns"
        :filter="search"
        :grid="$q.screen.lt.md"
        :loading="loading"
        :rows="rows"
        class="xy-standalone-table"
        flat
        hide-header-in-grid
        row-key="id">
        <template #item="props">
          <div class="game-grid-item col-12 col-sm-6">
            <q-card class="game-mobile-card" flat>
              <q-card-section class="game-mobile-header">
                <div class="game-mobile-identity">
                  <router-link :to="`/games/${props.row.id}/edit`" class="game-mobile-name">
                    {{ props.row.name }}
                  </router-link>
                  <q-badge
                    color="grey-8"
                    :label="props.row.xylonaOfficial ? 'Official' : 'Custom'" />
                </div>
              </q-card-section>

              <q-card-section class="game-mobile-details">
                <div>
                  <span>Game port</span>
                  <strong class="xy-num">{{ props.row.defaultPort || 'Not set' }}</strong>
                </div>
                <div>
                  <span>Query port</span>
                  <strong class="xy-num">{{ props.row.defaultQueryPort || 'Not set' }}</strong>
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
            <q-badge color="grey-8" :label="props.row.xylonaOfficial ? 'Official' : 'Custom'" />
          </q-td>
        </template>
        <template #body-cell-windows_support="props">
          <q-td :props="props">
            <span v-if="props.row.windowsSupport" aria-label="Supported" role="img">
              <q-icon name="check" size="sm" />
            </span>
            <span v-else aria-label="Not supported" class="text-xy-muted" role="img">—</span>
          </q-td>
        </template>
        <template #body-cell-linux_support="props">
          <q-td :props="props">
            <span v-if="props.row.linuxSupport" aria-label="Supported" role="img">
              <q-icon name="check" size="sm" />
            </span>
            <span v-else aria-label="Not supported" class="text-xy-muted" role="img">—</span>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="xy-row-actions">
              <q-btn
                :to="'/games/' + props.row.id + '/edit'"
                :aria-label="`Edit ${props.row.name}`"
                dense
                flat
                icon="edit"
                round>
                <q-tooltip>Edit game</q-tooltip>
              </q-btn>
              <q-btn
                :to="'/games/' + props.row.id + '/copy'"
                :aria-label="`Copy ${props.row.name}`"
                dense
                flat
                icon="content_copy"
                round>
                <q-tooltip>Copy game</q-tooltip>
              </q-btn>
              <q-btn
                :aria-label="`Export ${props.row.name} as JSON`"
                dense
                flat
                icon="file_download"
                round
                @click="exportGameAction(props.row)">
                <q-tooltip>Export game JSON</q-tooltip>
              </q-btn>
              <q-btn
                :aria-label="`Delete ${props.row.name}`"
                class="text-error-brighter"
                dense
                flat
                icon="delete"
                round
                @click="deleteGameAction(props.row)">
                <q-tooltip>Delete game</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <empty-state
            v-if="!loading && !loadError"
            :description="search ? 'Try a different search.' : 'Add a game to get started.'"
            :title="search ? 'No matching games' : 'No games yet'"
            icon="sports_esports">
            <template v-if="!search" #actions>
              <q-btn label="Add game" outline to="/games/new" />
            </template>
          </empty-state>
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
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { usePersistedRef } from '@/utils/persisted-ref'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import GameDeleteDialog from '@/components/games/GameDeleteDialog.vue'
import GameImportDialog from '@/components/games/GameImportDialog.vue'
import { exportGameDefinitionJSON } from '@/components/games/game-definition-json'
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
const initialPagination = usePersistedRef('game-pagination', {
  rowsPerPage: 25,
  page: 1,
  sortBy: 'name',
  descending: false,
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
    notifySuccess(`Exported ${fileName}.`, { icon: 'check_circle' })
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError, 'Failed to export game', { icon: 'report_problem' })
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
    label: 'Port',
    classes: 'xy-num',
    align: 'left',
    field: (row: { defaultPort: number }) => row.defaultPort,
    sortable: true,
  },
  {
    name: 'default_query_port',
    label: 'Query Port',
    classes: 'xy-num',
    align: 'left',
    field: (row: { defaultQueryPort: number }) => row.defaultQueryPort,
    sortable: true,
  },
  {
    name: 'windows_support',
    label: 'Windows',
    align: 'left',
    field: (row: { windowsSupport: boolean }) => row.windowsSupport,
    sortable: true,
  },
  {
    name: 'linux_support',
    label: 'Linux',
    align: 'left',
    field: (row: { linuxSupport: boolean }) => row.linuxSupport,
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center',
    field: () => '',
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
])
</script>

<style scoped>
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
