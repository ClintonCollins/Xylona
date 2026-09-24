<template>
  <q-page
    class="server-list-page xy-page-content"
    :class="{
      'server-list-page--with-settings': showStatusPageSettings,
      'server-list-page--selecting': selectedGameServers.length > 0,
    }">
    <page-header class="server-list-header" title="Game Servers">
      <div class="server-list-summary">
        <span>{{ displayRows.length }} {{ displayRows.length === 1 ? 'server' : 'servers' }}</span>
        <span aria-hidden="true">·</span>
        <span class="server-list-summary__online">{{ onlineServerCount }} online</span>
        <template v-if="totalPlayerCounts.max > 0">
          <span aria-hidden="true">·</span>
          <span> {{ totalPlayerCounts.current }} / {{ totalPlayerCounts.max }} players </span>
        </template>
      </div>
      <template #actions>
        <q-input
          v-if="displayRows.length > 0"
          v-model="search"
          aria-label="Search game servers"
          class="xy-search-input"
          color="primary"
          debounce="300"
          dense
          label="Search game servers"
          outlined
          placeholder="Name, game, node, or owner">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn
          :aria-label="$q.screen.xs ? 'Public status page' : undefined"
          :color="showStatusPageSettings ? 'primary' : undefined"
          flat
          icon="public"
          :label="$q.screen.xs ? undefined : 'Public status page'"
          no-caps
          :round="$q.screen.xs"
          @click="toggleStatusPageSettings">
          <q-tooltip v-if="$q.screen.xs">Public status page</q-tooltip>
        </q-btn>
        <q-btn
          v-if="showCreateButton && displayRows.length > 0"
          :disable="loading"
          color="primary"
          :label="$q.screen.xs ? 'Create' : 'Create Game Server'"
          to="/game-servers/create" />
      </template>
    </page-header>
    <div v-if="serverListError" class="server-list-error" role="alert" aria-live="assertive">
      <q-icon name="sync_problem" size="sm" />
      <div>
        <strong>Live server status could not be refreshed.</strong>
        <span>
          {{ serverListError }} Start, stop, restart, update, and delete remain unavailable.
        </span>
      </div>
      <q-btn :loading="loading" dense flat icon="refresh" label="Retry" @click="getGameServers" />
    </div>
    <div
      v-else-if="!lifecycleStateAuthoritative && !loading && !websocketConnectingQuietly"
      class="server-list-notice"
      role="status">
      <q-icon name="sync" size="sm" />
      <span>
        Connecting to live server status. Server controls are unavailable until the connection is
        ready.
      </span>
    </div>
    <div class="server-list-main">
      <div
        v-if="selectedGameServers.length > 0"
        aria-label="Selected game server actions"
        class="server-selection-bar"
        role="toolbar">
        <div class="server-selection-bar__count">
          <q-icon name="checklist" size="sm" />
          <strong>{{ selectedGameServers.length }} selected</strong>
        </div>
        <div class="server-selection-bar__actions">
          <q-btn
            v-for="action in serverActions"
            :key="action.name"
            :aria-label="`${action.label} ${selectedServersForAction[action.name].length} selected game servers`"
            :color="action.color"
            :disable="
              !lifecycleStateAuthoritative ||
              loading ||
              selectedServersForAction[action.name].length < 1
            "
            dense
            :icon="action.icon"
            :label="
              compactSelectionBar
                ? undefined
                : `${action.label} ${selectedServersForAction[action.name].length}`
            "
            no-caps
            outline
            @click="runSelectedServerAction(action.name)">
            <q-tooltip>{{ action.label }} selected game servers</q-tooltip>
          </q-btn>
          <q-btn
            :aria-label="`Delete ${selectedGameServers.length} selected game servers`"
            :disable="!lifecycleStateAuthoritative || loading"
            class="server-selection-bar__delete"
            color="negative"
            dense
            flat
            icon="delete_outline"
            :label="compactSelectionBar ? 'Delete' : `Delete ${selectedGameServers.length}`"
            no-caps
            @click="openDeleteDialog(selectedGameServers)">
            <q-tooltip>Delete selected game servers</q-tooltip>
          </q-btn>
          <q-btn
            aria-label="Clear server selection"
            dense
            flat
            icon="close"
            round
            @click="selectedGameServers = []">
            <q-tooltip>Clear selection</q-tooltip>
          </q-btn>
        </div>
      </div>
      <div v-if="gridMode && displayRows.length > 0" class="server-grid-controls">
        <q-checkbox
          :model-value="pageSelectionState"
          dense
          label="Select all"
          @update:model-value="togglePageSelection" />
        <div class="server-grid-controls__sort">
          <q-select
            v-model="gridSortBy"
            :options="gridSortOptions"
            class="server-grid-controls__sort-select"
            dense
            emit-value
            label="Sort by"
            map-options
            options-dense
            outlined />
          <q-btn
            :aria-label="initialPagination.descending ? 'Sort descending' : 'Sort ascending'"
            :disable="!gridSortBy"
            dense
            flat
            :icon="initialPagination.descending ? 'arrow_downward' : 'arrow_upward'"
            round
            @click="toggleSortDirection">
            <q-tooltip>{{ initialPagination.descending ? 'Descending' : 'Ascending' }}</q-tooltip>
          </q-btn>
        </div>
      </div>
      <q-table
        ref="serverTable"
        v-model:pagination="initialPagination"
        v-model:selected="selectedGameServers"
        aria-label="Game servers"
        :columns="columns"
        :filter="search"
        :grid="gridMode"
        :loading="loading"
        :rows="displayRows"
        class="xy-standalone-table"
        flat
        hide-selected-banner
        row-key="compositeId"
        selection="multiple">
        <template #item="props">
          <div class="server-grid-item col-12 col-sm-6">
            <q-card class="server-mobile-card" flat>
              <q-card-section class="server-mobile-header">
                <q-checkbox
                  v-model="props.selected"
                  :aria-label="`Select ${props.row.displayName}`"
                  class="server-mobile-select"
                  dense />
                <div class="server-mobile-identity">
                  <router-link
                    :to="`/game-servers/${props.row.id}/console`"
                    class="table-link server-mobile-name">
                    {{ props.row.displayName }}
                  </router-link>
                  <span>{{ props.row.gameName }}</span>
                </div>
                <status-badge
                  :phase="getStatusBadgePhase(props.row)"
                  :status="props.row.statusEnum" />
              </q-card-section>

              <q-separator />

              <q-card-section class="server-mobile-details">
                <div class="server-mobile-detail-group server-mobile-health">
                  <div>
                    <span class="server-mobile-label">Players</span>
                    <strong>{{ getPlayerCountLabel(props.row) }}</strong>
                  </div>
                  <div>
                    <span class="server-mobile-label">CPU</span>
                    <strong>{{ formatCpuUsage(props.row) }}</strong>
                  </div>
                  <div>
                    <span class="server-mobile-label">Memory</span>
                    <strong>{{ formatMemoryUsage(props.row) }}</strong>
                  </div>
                </div>
                <div class="server-mobile-context server-mobile-detail-group">
                  <div>
                    <span class="server-mobile-label">Node</span>
                    <strong>{{ props.row.nodeName }}</strong>
                  </div>
                  <div>
                    <span class="server-mobile-label">Version</span>
                    <strong>{{ getDisplayVersion(props.row) || 'Not reported' }}</strong>
                    <span
                      v-if="getVersionDisplay(props.row).updateAvailable"
                      class="server-mobile-update">
                      <span class="version-arrow" aria-hidden="true">→</span>
                      <span class="xy-visually-hidden">Update available:</span>
                      <span class="version-new">{{
                        getVersionDisplay(props.row).latestVersion
                      }}</span>
                    </span>
                  </div>
                  <div v-if="props.row.userName">
                    <span class="server-mobile-label">Owner</span>
                    <strong>{{ props.row.userName }}</strong>
                  </div>
                  <div>
                    <span class="server-mobile-label">Runtime</span>
                    <strong>{{ props.row.isLocal ? 'Local' : 'Remote' }}</strong>
                  </div>
                </div>
              </q-card-section>

              <q-card-actions class="server-mobile-actions">
                <q-btn
                  :to="`/game-servers/${props.row.id}/console`"
                  color="primary"
                  dense
                  flat
                  icon="terminal"
                  label="Console"
                  no-caps />
                <div class="server-mobile-row-actions">
                  <div class="server-lifecycle-actions">
                    <q-btn
                      v-for="action in serverActions"
                      :key="action.name"
                      :aria-label="`${action.label} ${props.row.displayName}`"
                      :disable="!canRunServerAction(props.row, action.name)"
                      :loading="isServerActionPending(props.row, action.name)"
                      :color="action.color"
                      dense
                      flat
                      :icon="action.icon"
                      round
                      @click="runServerAction(action.name, props.row)">
                      <q-tooltip>{{ getServerActionTooltip(props.row, action.name) }}</q-tooltip>
                    </q-btn>
                  </div>
                  <!-- Configure and Delete sit in a menu so Delete is never beside Console. -->
                  <q-btn
                    :aria-label="`More actions for ${props.row.displayName}`"
                    dense
                    flat
                    icon="more_vert"
                    round>
                    <q-tooltip>More actions</q-tooltip>
                    <q-menu anchor="bottom right" self="top right">
                      <q-list>
                        <q-item
                          v-close-popup
                          class="server-card-menu-item"
                          clickable
                          :to="`/game-servers/${props.row.id}/configuration`">
                          <q-item-section avatar>
                            <q-icon name="tune" />
                          </q-item-section>
                          <q-item-section>Configure</q-item-section>
                        </q-item>
                        <q-item
                          v-close-popup
                          class="server-card-menu-item server-card-menu-item--danger"
                          clickable
                          :disable="!lifecycleStateAuthoritative"
                          @click="openDeleteDialog([props.row])">
                          <q-item-section avatar>
                            <q-icon :name="tabTrash" />
                          </q-item-section>
                          <q-item-section>
                            <q-item-label>Delete…</q-item-label>
                            <q-item-label v-if="!lifecycleStateAuthoritative" caption>
                              Waiting for live server status
                            </q-item-label>
                          </q-item-section>
                        </q-item>
                      </q-list>
                    </q-menu>
                  </q-btn>
                </div>
              </q-card-actions>
            </q-card>
          </div>
        </template>
        <template #body-selection="scope">
          <q-checkbox v-model="scope.selected" :aria-label="`Select ${scope.row.displayName}`" />
        </template>
        <template #body-cell-name="props">
          <q-td :props="props">
            <router-link :to="'/game-servers/' + props.row.id + '/console'" class="table-link">
              {{ props.row.displayName }}
            </router-link>
            <q-badge v-if="props.row.isStale" class="q-ml-xs" color="warning" label="stale">
              <span class="xy-visually-hidden">
                : node {{ props.row.nodeName }} has not reported recently, so this status may be out
                of date.
              </span>
              <q-tooltip>
                Node {{ props.row.nodeName }} has not reported recently. This status may be out of
                date.
              </q-tooltip>
            </q-badge>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <status-badge
              :phase="getStatusBadgePhase(props.row)"
              :status="props.row.statusEnum"></status-badge>
          </q-td>
        </template>
        <template #body-cell-players="props">
          <q-td :props="props">
            <span class="server-player-count">
              <q-icon name="group" size="1rem" />
              {{ getPlayerCountLabel(props.row) }}
            </span>
          </q-td>
        </template>
        <template #body-cell-resources="props">
          <q-td :props="props">
            <div class="server-resource-usage">
              <span><q-icon name="memory" /> CPU {{ formatCpuUsage(props.row) }}</span>
              <span><q-icon name="developer_board" /> RAM {{ formatMemoryUsage(props.row) }}</span>
            </div>
          </q-td>
        </template>
        <template #body-cell-version="props">
          <q-td :props="props">
            <template v-if="getVersionDisplay(props.row).checked">
              <span class="version-text">{{ getVersionDisplay(props.row).installedVersion }}</span>
              <template v-if="getVersionDisplay(props.row).updateAvailable">
                <span class="version-arrow">→</span>
                <span class="version-new">{{ getVersionDisplay(props.row).latestVersion }}</span>
              </template>
            </template>
            <template v-else-if="getVersionDisplay(props.row).checking">
              <q-spinner color="primary" size="1em" />
            </template>
            <template v-else-if="getVersionDisplay(props.row).installedVersion">
              <span class="version-text">{{ getVersionDisplay(props.row).installedVersion }}</span>
            </template>
            <template v-else>
              <span class="version-na">—</span>
            </template>
          </q-td>
        </template>
        <template #body-cell-node="props">
          <q-td :props="props">
            <span>{{ props.row.nodeName }}</span>
            <q-badge
              class="q-ml-xs"
              color="grey-8"
              :label="props.row.isLocal ? 'local' : 'remote'" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="server-table-actions">
              <div class="server-lifecycle-actions">
                <q-btn
                  v-for="action in serverActions"
                  :key="action.name"
                  :aria-label="`${action.label} ${props.row.displayName}`"
                  :disable="!canRunServerAction(props.row, action.name)"
                  :loading="isServerActionPending(props.row, action.name)"
                  :color="action.color"
                  dense
                  flat
                  :icon="action.icon"
                  round
                  @click="runServerAction(action.name, props.row)">
                  <q-tooltip>{{ getServerActionTooltip(props.row, action.name) }}</q-tooltip>
                </q-btn>
              </div>
              <q-separator vertical />
              <q-btn
                :to="'/game-servers/' + props.row.id + '/configuration'"
                :aria-label="`Configure ${props.row.displayName}`"
                dense
                flat
                icon="tune"
                round>
                <q-tooltip>Configure {{ props.row.displayName }}</q-tooltip>
              </q-btn>
              <span>
                <q-btn
                  :icon="tabTrash"
                  :disable="!lifecycleStateAuthoritative"
                  :aria-label="`Delete ${props.row.displayName}`"
                  class="text-error-brighter"
                  dense
                  flat
                  round
                  @click="openDeleteDialog([props.row])">
                  <q-tooltip>
                    {{
                      lifecycleStateAuthoritative
                        ? `Delete ${props.row.displayName}`
                        : `Live status is unavailable; ${props.row.displayName} cannot be deleted yet.`
                    }}
                  </q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <empty-state
            v-if="!loading && !serverListError"
            :icon="search.trim().length > 0 ? 'search_off' : 'dns'"
            :title="search.trim().length > 0 ? 'No matching game servers' : 'No game servers'">
            <template v-if="search.trim().length > 0">
              No game servers match “{{ search.trim() }}”.
            </template>
            <template v-else>Create a game server to get started.</template>
            <template v-if="search.trim().length > 0 || showCreateButton" #actions>
              <q-btn
                v-if="search.trim().length > 0"
                flat
                label="Clear search"
                @click="search = ''" />
              <q-btn v-else color="primary" label="Create Game Server" to="/game-servers/create" />
            </template>
          </empty-state>
        </template>
      </q-table>
    </div>
    <game-server-status-page-settings-panel
      v-if="showStatusPageSettings"
      ref="statusPageSettingsPanel"
      class="server-status-panel"
      @close="showStatusPageSettings = false" />
    <delete-game-server-dialog
      v-model:show-dialog="showDeleteGameServerDialog"
      :game-servers="deleteTargets"
      @submit="deleteGameServerSubmitted"></delete-game-server-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, nextTick, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import {
  ConnectErrorToString,
  getLatestServersQueryInfo,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { isServerStopping } from '@/utils/game-server-stopping'
import { createServerMetricsSubscriptions } from '@/utils/server-metrics-subscriptions'
import DeleteGameServerDialog, {
  type DeleteGameServerTarget,
} from '@/components/game_servers/DeleteGameServerDialog.vue'
import GameServerStatusPageSettingsPanel from '@/components/game_servers/GameServerStatusPageSettingsPanel.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import type { StepState } from '@/components/game_servers/UpdateProgressPanel.types'
import StatusBadge, { type StatusBadgePhase } from '@/components/StatusBadge.vue'
import {
  type AllServersQueryInfo,
  Node,
  StartGameServerRequest,
  StartGameServerRequestSchema,
  Status,
  RestartGameServerRequest,
  RestartGameServerRequestSchema,
  StopGameServerRequest,
  StopGameServerRequestSchema,
  type VersionInfo,
} from '@/proto/shared_pb'
import { usePersistedRef } from '@/utils/persisted-ref'
import {
  AggregatedGameServer,
  ListAggregatedGameServersRequestSchema,
  ListNodesRequestSchema,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema,
  type UpdateProgress,
} from '@/proto/xylona_pb'
import { type AllServersMetrics } from '@/proto/websocket_pb'
import { buildDisplayRows, type DisplayRow } from './server-list-cache'
import { queryInfoPlayerSnapshot } from './useGameServerQueryStatusVersion'
import {
  askLifecycleConfirmation,
  buildLifecycleConfirmation,
  canRestartServer,
  canStartServer,
  canStopServer,
  canUpdateServer,
  getRestartableServers,
  getStartableServers,
  getStoppableServers,
  getUpdateableServers,
  isServerRunning,
  type LifecycleConfirmAction,
} from './server-list-actions'
import { useUserAuthStore } from '@/stores/xylona'
import { resolveCanonicalVersionDisplay } from './version-display'
import {
  websocketConnectingQuietly,
  websocketStateAuthoritative,
} from '@/utils/websocket-connection'
import { formatMetricBytes } from './metrics-format'
import { recordLifecycleIntent } from '@/utils/game-server-notifications'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import { applyUpdateProgress, buildUpdateSteps, isUpdateProgressTerminal } from './update-progress'

const aggregatedServers = ref<AggregatedGameServer[] | null>(null)
const nodesByID = ref(new Map<string, Node>())
const serverStatusSnapshotFresh = ref(false)
const serverListError = ref('')
const lifecycleStateAuthoritative = computed(
  () => websocketStateAuthoritative.value && serverStatusSnapshotFresh.value,
)
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const showStatusPageSettings = ref(false)
const statusPageSettingsPanel = ref<InstanceType<typeof GameServerStatusPageSettingsPanel> | null>(
  null,
)

async function toggleStatusPageSettings() {
  if (!showStatusPageSettings.value) {
    showStatusPageSettings.value = true
    // Below 1024px the panel renders under the whole list, so bring it into view.
    if ($q.screen.lt.md) {
      await nextTick()
      const panel = statusPageSettingsPanel.value?.$el as HTMLElement | undefined
      panel?.scrollIntoView({ block: 'start' })
    }
    return
  }
  // Closing goes through the panel so unsaved edits get the discard prompt.
  await statusPageSettingsPanel.value?.requestClose()
}
const selectedGameServers = ref([] as DisplayRow[])
type ServerAction = 'start' | 'stop' | 'restart' | 'update'
const serverActions: readonly {
  name: ServerAction
  label: string
  color: string
  icon: string
}[] = [
  { name: 'start', label: 'Start', color: 'positive', icon: 'play_arrow' },
  { name: 'restart', label: 'Restart', color: 'warning', icon: 'restart_alt' },
  { name: 'stop', label: 'Stop', color: 'negative', icon: 'stop' },
  { name: 'update', label: 'Update', color: 'accent', icon: 'system_update_alt' },
]
// current is null while a server is up but has not answered its player query.
type ServerPlayerCounts = { current: number | null; max: number }
type ServerResourceUsage = {
  cpuPercent: number | null
  memoryBytes: number | null
  memoryPercent: number | null
}
const pendingActionByServerID = ref(new Map<string, ServerAction>())
const updateStepsByServerID = new Map<string, StepState[]>()
const playerCountsByServerID = ref(new Map<string, ServerPlayerCounts>())
const resourceUsageByServerID = ref(new Map<string, ServerResourceUsage>())
const $q = useQuasar()
let loadSequence = 0
let initialLoadComplete = false
let reconnectRefreshQueued = false
let serverListUnmounted = false
const metricsSubscriptions = createServerMetricsSubscriptions()
type BufferedLiveServerState = {
  status?: Status
  version?: string
  versionInfo?: VersionInfo
}
const bufferedLiveServerStateByID = new Map<string, BufferedLiveServerState>()

const initialPagination = usePersistedRef('game-server-pagination', {
  rowsPerPage: 25,
  page: 1,
  sortBy: null as string | null,
  descending: false,
})
const authStore = useUserAuthStore()
const showCreateButton = computed(() => authStore.user?.superUser ?? false)

const displayRows = computed((): DisplayRow[] => {
  return buildDisplayRows(aggregatedServers.value ?? [], nodesByID.value)
})

const onlineServerCount = computed(
  () => displayRows.value.filter((server) => server.statusEnum === Status.ONLINE).length,
)

// Servers with an unknown count are left out rather than counted as empty.
const totalPlayerCounts = computed(() => {
  return displayRows.value.reduce(
    (total, server) => {
      const counts = getPlayerCounts(server)
      if (counts.current === null) return total
      total.current += counts.current
      total.max += counts.max
      return total
    },
    { current: 0, max: 0 },
  )
})

// The dialog's own targets, so a row's Delete never replaces the multi-selection.
const deleteTargets = ref<DeleteGameServerTarget[]>([])

const selectedGameServersForStart = computed(() => {
  return getStartableServers(selectedGameServers.value).filter(
    (server) => hasPermission(server, 'game_server.start') && !isServerActionPending(server),
  )
})

const selectedGameServersForStop = computed(() => {
  return getStoppableServers(selectedGameServers.value).filter(
    (server) => hasPermission(server, 'game_server.stop') && !isServerActionPending(server),
  )
})

const selectedGameServersForRestart = computed(() => {
  return getRestartableServers(selectedGameServers.value).filter(
    (server) => hasPermission(server, 'game_server.restart') && !isServerActionPending(server),
  )
})

const selectedGameServersForUpdate = computed(() => {
  return getUpdateableServers(selectedGameServers.value).filter(
    (server) => hasPermission(server, 'game_server.settings') && !isServerActionPending(server),
  )
})

const selectedServersForAction = computed((): Record<ServerAction, DisplayRow[]> => ({
  start: selectedGameServersForStart.value,
  restart: selectedGameServersForRestart.value,
  stop: selectedGameServersForStop.value,
  update: selectedGameServersForUpdate.value,
}))

// Bulk buttons drop their "Start 2" labels below 1024px; Delete always keeps its label.
const compactSelectionBar = computed(() => $q.screen.lt.md)

// Cards below Quasar's lg step (1440px). A media query counts the page scrollbar in the
// width, so a scrollbar can't flip a 1440px window to cards the way $q.screen.lt.lg did.
const belowLargeQuery =
  typeof window.matchMedia === 'function' ? window.matchMedia('(max-width: 1439px)') : null
const gridMode = ref(belowLargeQuery?.matches ?? false)
function syncGridMode(event: MediaQueryListEvent) {
  gridMode.value = event.matches
}

// The card grid has no table header, so it gets its own select-all and sort controls.
const serverTable = ref<{ computedRows: DisplayRow[] } | null>(null)
const pageSelectionState = computed((): boolean | null => {
  const pageRows = serverTable.value?.computedRows ?? []
  const selectedKeys = new Set(selectedGameServers.value.map((row) => row.compositeId))
  const selectedOnPage = pageRows.filter((row) => selectedKeys.has(row.compositeId)).length
  if (selectedOnPage === 0) return false
  return selectedOnPage === pageRows.length ? true : null
})

function togglePageSelection() {
  const pageRows = serverTable.value?.computedRows ?? []
  const pageKeys = new Set(pageRows.map((row) => row.compositeId))
  const others = selectedGameServers.value.filter((row) => !pageKeys.has(row.compositeId))
  selectedGameServers.value = pageSelectionState.value === true ? others : [...others, ...pageRows]
}

const gridSortOptions = computed(() =>
  columns.value
    .filter((column) => 'sortable' in column && column.sortable)
    .map((column) => ({ label: column.label, value: column.name })),
)
const gridSortBy = computed({
  get: () => initialPagination.value.sortBy ?? null,
  set: (sortBy: string | null) => {
    initialPagination.value = { ...initialPagination.value, sortBy }
  },
})

function toggleSortDirection() {
  initialPagination.value = {
    ...initialPagination.value,
    descending: !initialPagination.value.descending,
  }
}

function hasPermission(server: DisplayRow, permission: string): boolean {
  return (
    authStore.user?.superUser === true || (server.effectivePermissions ?? []).includes(permission)
  )
}

function isServerActionPending(server: DisplayRow, action?: ServerAction): boolean {
  const pendingAction = pendingActionByServerID.value.get(server.id)
  return action === undefined ? pendingAction !== undefined : pendingAction === action
}

function canRunServerAction(server: DisplayRow, action: ServerAction): boolean {
  if (!lifecycleStateAuthoritative.value || loading.value || isServerActionPending(server)) {
    return false
  }

  switch (action) {
    case 'start':
      return canStartServer(server.statusEnum) && hasPermission(server, 'game_server.start')
    case 'stop':
      return (
        canStopServer(server.statusEnum) &&
        !isServerStopping(server.id, server.statusEnum) &&
        hasPermission(server, 'game_server.stop')
      )
    case 'restart':
      return (
        canRestartServer(server.statusEnum) &&
        !isServerStopping(server.id, server.statusEnum) &&
        hasPermission(server, 'game_server.restart')
      )
    case 'update':
      return canUpdateServer(server) && hasPermission(server, 'game_server.settings')
  }
}

function getServerActionTooltip(server: DisplayRow, action: ServerAction): string {
  if (!lifecycleStateAuthoritative.value) {
    return 'Waiting for authoritative server status'
  }
  const pendingAction = pendingActionByServerID.value.get(server.id)
  if (pendingAction !== undefined) {
    return `${pendingAction[0]?.toUpperCase()}${pendingAction.slice(1)} is in progress`
  }

  const requiredPermissions: Record<ServerAction, string[]> = {
    start: ['game_server.start'],
    stop: ['game_server.stop'],
    restart: ['game_server.restart'],
    update: ['game_server.settings'],
  }
  if (requiredPermissions[action].some((permission) => !hasPermission(server, permission))) {
    return `You do not have permission to ${action} this server`
  }

  if (action === 'update' && !server.canUpdate) {
    return 'This server does not have an update provider'
  }
  if (action === 'start' && server.statusEnum !== Status.OFFLINE) {
    return 'Start is available when the server is offline'
  }
  if (
    (action === 'stop' || action === 'restart') &&
    isServerStopping(server.id, server.statusEnum)
  ) {
    return 'The server is stopping'
  }
  if ((action === 'stop' || action === 'restart') && !isServerRunning(server.statusEnum)) {
    return `${action === 'stop' ? 'Stop' : 'Restart'} is available when the server is running`
  }
  if (
    action === 'update' &&
    !isServerRunning(server.statusEnum) &&
    server.statusEnum !== Status.OFFLINE
  ) {
    return 'Update is unavailable while another operation is running'
  }

  return `${action[0]?.toUpperCase()}${action.slice(1)} ${server.displayName}`
}

// Live counts and usage come from the websocket feed, so a list refresh after a
// lifecycle action does not blank them the way it pauses the controls.
function getPlayerCounts(server: DisplayRow): ServerPlayerCounts {
  const liveCounts = websocketStateAuthoritative.value
    ? playerCountsByServerID.value.get(server.id)
    : undefined
  const max = liveCounts?.max || server.maxPlayers || 0
  if (server.statusEnum !== Status.ONLINE || !websocketStateAuthoritative.value) {
    return { current: 0, max }
  }
  // No answered query means the count is unknown, not zero.
  return { current: liveCounts?.current ?? null, max }
}

function getPlayerCountLabel(server: DisplayRow): string {
  const counts = getPlayerCounts(server)
  const current = counts.current ?? 'Unknown'
  return counts.max > 0 ? `${current} / ${counts.max}` : `${current}`
}

function getStatusBadgePhase(server: DisplayRow): StatusBadgePhase | undefined {
  const pendingAction = pendingActionByServerID.value.get(server.id)
  if (pendingAction === 'restart') return 'restarting'
  if (pendingAction === 'stop' || isServerStopping(server.id, server.statusEnum)) return 'stopping'
  return undefined
}

function getResourceUsage(server: DisplayRow): ServerResourceUsage {
  if (
    !websocketStateAuthoritative.value ||
    !hasPermission(server, 'game_server.metrics') ||
    !isServerRunning(server.statusEnum)
  ) {
    return { cpuPercent: null, memoryBytes: null, memoryPercent: null }
  }

  return (
    resourceUsageByServerID.value.get(server.id) ?? {
      cpuPercent: server.cpuPercent ?? null,
      memoryBytes: server.memoryBytes ?? null,
      memoryPercent: server.memoryPercent ?? null,
    }
  )
}

function formatCpuUsage(server: DisplayRow): string {
  const cpuPercent = getResourceUsage(server).cpuPercent
  return cpuPercent === null ? '—' : `${cpuPercent.toFixed(1)}%`
}

function formatMemoryUsage(server: DisplayRow): string {
  const usage = getResourceUsage(server)
  if (usage.memoryBytes === null) {
    return '—'
  }

  const bytes = formatMetricBytes(usage.memoryBytes)
  return usage.memoryPercent === null ? bytes : `${bytes} · ${usage.memoryPercent.toFixed(1)}%`
}

function applyServerQueryInfo(queryInfo: AllServersQueryInfo) {
  const nextCounts = new Map(playerCountsByServerID.value)
  for (const [serverID, serverQuery] of Object.entries(queryInfo.servers)) {
    const server = displayRows.value.find((row) => row.id === serverID)
    if (!server || server.statusEnum !== Status.ONLINE) {
      nextCounts.delete(serverID)
      continue
    }
    const snapshot = queryInfoPlayerSnapshot(serverQuery)
    if (snapshot !== null) {
      nextCounts.set(serverID, {
        current: snapshot.responded ? snapshot.playerCount : null,
        max: snapshot.playerCapacity,
      })
    }
  }
  playerCountsByServerID.value = nextCounts
}

function applyServerMetrics(metrics: AllServersMetrics) {
  const nextUsage = new Map(resourceUsageByServerID.value)
  for (const [serverID, serverMetrics] of Object.entries(metrics.servers)) {
    const server = displayRows.value.find((row) => row.id === serverID)
    if (!server || !isServerRunning(server.statusEnum)) {
      nextUsage.delete(serverID)
      continue
    }
    if (!serverMetrics.metricsValid) {
      nextUsage.set(serverID, {
        cpuPercent: null,
        memoryBytes: null,
        memoryPercent: null,
      })
      continue
    }

    const workingSetBytes = Number(serverMetrics.memoryWorkingSetBytes)
    nextUsage.set(serverID, {
      cpuPercent: serverMetrics.cpuValid ? serverMetrics.cpuPercent : null,
      memoryBytes: workingSetBytes > 0 ? workingSetBytes : Number(serverMetrics.memoryBytes),
      memoryPercent: Number.isFinite(serverMetrics.memoryPercent)
        ? serverMetrics.memoryPercent
        : null,
    })
  }
  resourceUsageByServerID.value = nextUsage
}

function applyNodesResponse(nodes: Node[]) {
  nodesByID.value = new Map(nodes.map((node) => [node.id, node]))
}

function recordBufferedLiveServerState(serverID: string, state: BufferedLiveServerState) {
  const existingState = bufferedLiveServerStateByID.get(serverID) ?? {}
  bufferedLiveServerStateByID.set(serverID, {
    ...existingState,
    ...state,
  })
}

function applyBufferedLiveServerStateToServers(
  servers: AggregatedGameServer[],
): AggregatedGameServer[] {
  for (const server of servers) {
    if (server.isLocal && server.localServer) {
      const bufferedState = bufferedLiveServerStateByID.get(server.localServer.id)
      if (!bufferedState) {
        continue
      }

      if (
        Object.prototype.hasOwnProperty.call(bufferedState, 'status') &&
        bufferedState.status !== undefined
      ) {
        server.localServer.status = bufferedState.status
      }
      if (Object.prototype.hasOwnProperty.call(bufferedState, 'version')) {
        server.localServer.version = bufferedState.version ?? ''
      }
      if (Object.prototype.hasOwnProperty.call(bufferedState, 'versionInfo')) {
        server.localServer.versionInfo = bufferedState.versionInfo
      }
      continue
    }

    if (!server.isLocal && server.remoteServer) {
      const bufferedState = bufferedLiveServerStateByID.get(server.remoteServer.remoteServerId)
      if (!bufferedState) {
        continue
      }

      if (
        Object.prototype.hasOwnProperty.call(bufferedState, 'status') &&
        bufferedState.status !== undefined
      ) {
        server.remoteServer.status = bufferedState.status
      }
      if (Object.prototype.hasOwnProperty.call(bufferedState, 'version')) {
        server.remoteServer.version = bufferedState.version ?? ''
      }
      if (Object.prototype.hasOwnProperty.call(bufferedState, 'versionInfo')) {
        server.remoteServer.versionInfo = bufferedState.versionInfo
      }
    }
  }

  return servers
}

onMounted(async () => {
  belowLargeQuery?.addEventListener('change', syncGridMode)
  watchServerStatusChanges()
  watchServerVersionChanges()
  watchWebsocketReconnects()
  XylonaEventBus.on('gameServersQueryInfo', applyServerQueryInfo)
  XylonaEventBus.on('gameServerMetrics', applyServerMetrics)
  XylonaEventBus.on('gameServerUpdateProgress', handleGameServerUpdateProgress)
  await getGameServers()
  if (serverListUnmounted) {
    return
  }
  const latestQueryInfo = getLatestServersQueryInfo()
  if (latestQueryInfo !== undefined) applyServerQueryInfo(latestQueryInfo)
  initialLoadComplete = true
  runQueuedReconnectRefresh()
})

onBeforeUnmount(() => {
  serverListUnmounted = true
  reconnectRefreshQueued = false
  XylonaEventBus.off('gameServerStatus', handleServerStatusUpdate)
  XylonaEventBus.off('gameServerVersion', handleServerVersionUpdate)
  XylonaEventBus.off('websocketConnected', handleWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', handleWebsocketDisconnect)
  XylonaEventBus.off('gameServersQueryInfo', applyServerQueryInfo)
  XylonaEventBus.off('gameServerMetrics', applyServerMetrics)
  XylonaEventBus.off('gameServerUpdateProgress', handleGameServerUpdateProgress)
  belowLargeQuery?.removeEventListener('change', syncGridMode)
  metricsSubscriptions.clear()
})

async function getGameServers() {
  const loadID = ++loadSequence
  serverStatusSnapshotFresh.value = false
  // Keep the current rows on screen until the response replaces them.
  bufferedLiveServerStateByID.clear()
  loading.value = true
  const xylonaClient = GetXylonaClient()

  const aggregatedRequest = xylonaClient
    .listAggregatedGameServers(create(ListAggregatedGameServersRequestSchema, {}))
    .then((response) => {
      if (loadID !== loadSequence) {
        return
      }

      const servers = applyBufferedLiveServerStateToServers(response.servers)
      aggregatedServers.value = servers
      metricsSubscriptions.sync(
        buildDisplayRows(servers, nodesByID.value)
          .filter((server) => hasPermission(server, 'game_server.metrics'))
          .map((server) => server.id),
      )
      serverStatusSnapshotFresh.value = websocketStateAuthoritative.value
      serverListError.value = ''
    })
    .catch((reason: unknown) => {
      if (loadID !== loadSequence) {
        return
      }

      console.error(reason)
      serverListError.value = ConnectErrorToString(ConnectError.from(reason))
      notifyConnectError(reason, 'Failed to load game servers')
    })
    .finally(() => {
      if (loadID !== loadSequence) {
        return
      }
      loading.value = false
      runQueuedReconnectRefresh()
    })
  const nodesRequest = xylonaClient
    .listNodes(create(ListNodesRequestSchema, {}))
    .then((response) => {
      if (loadID !== loadSequence) {
        return
      }

      applyNodesResponse(response.nodes)
    })
    .catch((reason: unknown) => {
      if (loadID !== loadSequence) {
        return
      }

      console.error(reason)
      notifyConnectError(reason, 'Failed to load nodes')
      nodesByID.value = new Map()
    })

  void nodesRequest

  await aggregatedRequest
}

function watchServerStatusChanges() {
  XylonaEventBus.on('gameServerStatus', handleServerStatusUpdate)
}

function watchServerVersionChanges() {
  XylonaEventBus.on('gameServerVersion', handleServerVersionUpdate)
}

function watchWebsocketReconnects() {
  XylonaEventBus.on('websocketConnected', handleWebsocketReconnect)
  XylonaEventBus.on('websocketDisconnected', handleWebsocketDisconnect)
}

function handleWebsocketDisconnect() {
  serverStatusSnapshotFresh.value = false
  metricsSubscriptions.forget()
  playerCountsByServerID.value = new Map()
  resourceUsageByServerID.value = new Map()
  updateStepsByServerID.clear()

  const nextPendingActions = new Map(pendingActionByServerID.value)
  for (const [serverID, action] of nextPendingActions) {
    if (action === 'update') {
      nextPendingActions.delete(serverID)
    }
  }
  pendingActionByServerID.value = nextPendingActions
}

function handleWebsocketReconnect() {
  if (!initialLoadComplete || loading.value) {
    reconnectRefreshQueued = true
    return
  }

  void getGameServers()
}

function runQueuedReconnectRefresh() {
  if (serverListUnmounted || !initialLoadComplete || loading.value || !reconnectRefreshQueued) {
    return
  }

  reconnectRefreshQueued = false
  void getGameServers()
}

function handleGameServerUpdateProgress(progress: UpdateProgress) {
  const currentSteps =
    updateStepsByServerID.get(progress.gameServerId) ?? buildUpdateSteps(Status.UNKNOWN)
  const nextSteps = applyUpdateProgress(currentSteps, progress)
  updateStepsByServerID.set(progress.gameServerId, nextSteps)
  if (!isUpdateProgressTerminal(progress, nextSteps)) {
    return
  }

  updateStepsByServerID.delete(progress.gameServerId)
  const nextPendingActions = new Map(pendingActionByServerID.value)
  if (nextPendingActions.get(progress.gameServerId) === 'update') {
    nextPendingActions.delete(progress.gameServerId)
    pendingActionByServerID.value = nextPendingActions
  }
  void getGameServers()
}

function openDeleteDialog(rows: DisplayRow[]) {
  if (!lifecycleStateAuthoritative.value) {
    return
  }
  deleteTargets.value = rows.map((row) => ({
    id: row.id,
    name: row.displayName,
    nodeName: row.nodeName,
    directory: row.directory,
    canDeleteBackups: hasPermission(row, 'game_server.backup'),
  }))
  showDeleteGameServerDialog.value = true
}

async function deleteGameServerSubmitted(result: {
  succeeded: Array<{ id: string; name: string }>
  failed: Array<{ id: string; name: string; error: string }>
}) {
  showDeleteGameServerDialog.value = false
  if (result.succeeded.length > 0) {
    await getGameServers()
  }
  // Keep the rest of the selection; deleted servers leave it even if the refetch was
  // superseded or failed and the rows are still showing.
  const keptKeys = new Set(selectedGameServers.value.map((row) => row.compositeId))
  const deletedIDs = new Set(result.succeeded.map((server) => server.id))
  selectedGameServers.value = displayRows.value.filter(
    (row) => keptKeys.has(row.compositeId) && !deletedIDs.has(row.id),
  )
}

function setServerStatus(serverID: string, serverStatus: Status) {
  recordBufferedLiveServerState(serverID, { status: serverStatus })

  if (serverStatus !== Status.ONLINE) {
    const nextPlayerCounts = new Map(playerCountsByServerID.value)
    nextPlayerCounts.delete(serverID)
    playerCountsByServerID.value = nextPlayerCounts
  }
  if (!isServerRunning(serverStatus)) {
    const nextResourceUsage = new Map(resourceUsageByServerID.value)
    nextResourceUsage.delete(serverID)
    resourceUsageByServerID.value = nextResourceUsage
  }

  updateLiveServerData((server) => {
    if (server.isLocal && server.localServer && server.localServer.id === serverID) {
      server.localServer.status = serverStatus
      if (serverStatus !== Status.ONLINE) {
        server.localServer.currentPlayerCount = 0n
      }
      return true
    }
    if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
      server.remoteServer.status = serverStatus
      if (serverStatus !== Status.ONLINE) {
        server.remoteServer.currentPlayers = 0n
      }
      return true
    }
    return false
  })
}

function setServerVersion(serverID: string, version: string, versionInfo?: VersionInfo) {
  recordBufferedLiveServerState(serverID, { version, versionInfo })

  updateLiveServerData((server) => {
    if (server.isLocal && server.localServer && server.localServer.id === serverID) {
      server.localServer.version = version
      server.localServer.versionInfo = versionInfo
      return true
    }
    if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
      server.remoteServer.version = version
      server.remoteServer.versionInfo = versionInfo
      return true
    }
    return false
  })
}

function updateLiveServerData(updater: (server: AggregatedGameServer) => boolean) {
  const liveServerSets = [aggregatedServers.value]

  for (const liveServers of liveServerSets) {
    if (liveServers === null) {
      continue
    }

    for (const server of liveServers) {
      const didUpdate = updater(server)
      if (!didUpdate) {
        continue
      }
    }
  }
}

function handleServerStatusUpdate(serverID: string, _serverName: string, serverStatus: Status) {
  setServerStatus(serverID, serverStatus)
}

function handleServerVersionUpdate(serverID: string, version: string, versionInfo?: VersionInfo) {
  setServerVersion(serverID, version, versionInfo)
}

function getVersionDisplay(row: DisplayRow) {
  return resolveCanonicalVersionDisplay(row.version, row.versionInfo)
}

function getDisplayVersion(row: DisplayRow): string {
  return getVersionDisplay(row).installedVersion
}

function getEligibleServers(action: ServerAction, servers: DisplayRow[]): DisplayRow[] {
  switch (action) {
    case 'start':
      return getStartableServers(servers).filter((server) => canRunServerAction(server, action))
    case 'stop':
      return getStoppableServers(servers).filter((server) => canRunServerAction(server, action))
    case 'restart':
      return getRestartableServers(servers).filter((server) => canRunServerAction(server, action))
    case 'update':
      return getUpdateableServers(servers).filter((server) => canRunServerAction(server, action))
  }
}

async function confirmUpdateServers(servers: DisplayRow[]): Promise<boolean> {
  const runningCount = servers.filter((server) => isServerRunning(server.statusEnum)).length
  if (runningCount === 0) {
    return true
  }

  return new Promise((resolve) => {
    let settled = false
    $q.dialog({
      title: `Update ${servers.length === 1 ? 'server' : `${servers.length} servers`}?`,
      message:
        servers.length === 1
          ? 'Xylona will stop the server, install the update, and start it again.'
          : `Xylona will update all selected servers in parallel. ${runningCount} running ${runningCount === 1 ? 'server' : 'servers'} will be stopped and started again.`,
      cancel: true,
      persistent: true,
      ok: {
        label: servers.length === 1 ? 'Update server' : 'Update servers',
        color: 'primary',
        unelevated: true,
      },
    })
      .onOk(() => {
        settled = true
        resolve(true)
      })
      .onDismiss(() => {
        if (!settled) {
          resolve(false)
        }
      })
  })
}

function confirmLifecycleServers(
  action: LifecycleConfirmAction,
  servers: DisplayRow[],
): Promise<boolean> {
  return askLifecycleConfirmation(
    $q,
    buildLifecycleConfirmation(
      action,
      servers.map((server) => ({
        displayName: server.displayName,
        playerCount: getPlayerCounts(server).current,
      })),
    ),
  )
}

function setPendingActions(servers: DisplayRow[], action?: ServerAction) {
  const nextPendingActions = new Map(pendingActionByServerID.value)
  for (const server of servers) {
    if (action === undefined) {
      nextPendingActions.delete(server.id)
    } else {
      nextPendingActions.set(server.id, action)
    }
  }
  pendingActionByServerID.value = nextPendingActions
}

async function executeServerAction(action: ServerAction, server: DisplayRow) {
  const client = GetXylonaClient()
  switch (action) {
    case 'start': {
      const request: StartGameServerRequest = create(StartGameServerRequestSchema, {
        serverId: server.id,
      })
      await client.startGameServer(request)
      return
    }
    case 'stop': {
      const request: StopGameServerRequest = create(StopGameServerRequestSchema, {
        serverId: server.id,
      })
      await client.stopGameServer(request)
      return
    }
    case 'restart': {
      const request: RestartGameServerRequest = create(RestartGameServerRequestSchema, {
        serverId: server.id,
      })
      await client.restartGameServer(request)
      return
    }
    case 'update': {
      updateStepsByServerID.set(server.id, buildUpdateSteps(server.statusEnum))
      const request: UpdateGameServerRequest = create(UpdateGameServerRequestSchema, {
        serverId: server.id,
      })
      await client.updateGameServer(request)
      recordLifecycleIntent(server.id, 'update')
    }
  }
}

async function runServerActions(
  action: ServerAction,
  requestedServers: DisplayRow[],
  updateSelection = false,
) {
  const servers = getEligibleServers(action, requestedServers)
  if (servers.length === 0) {
    return
  }
  if (action === 'update' && !(await confirmUpdateServers(servers))) {
    return
  }
  if (
    (action === 'stop' || action === 'restart') &&
    !(await confirmLifecycleServers(action, servers))
  ) {
    return
  }

  setPendingActions(servers, action)
  const results = await Promise.all(
    servers.map(async (server) => {
      try {
        await executeServerAction(action, server)
        return { server, error: '' }
      } catch (error) {
        console.error(error)
        return {
          server,
          error: ConnectErrorToString(ConnectError.from(error)),
        }
      }
    }),
  )
  const failedResults = results.filter((result) => result.error !== '')
  if (action === 'update') {
    const failedServers = failedResults.map((result) => result.server)
    setPendingActions(failedServers)
    for (const server of failedServers) updateStepsByServerID.delete(server.id)
  } else {
    setPendingActions(servers)
  }
  if (updateSelection) {
    const failedServerIDs = new Set(failedResults.map((result) => result.server.id))
    selectedGameServers.value = selectedGameServers.value.filter((server) =>
      failedServerIDs.has(server.id),
    )
  }

  if (failedResults.length > 0) {
    const details = failedResults
      .map((result) => `${result.server.displayName}: ${result.error}`)
      .join('; ')
    notifyError(
      `Could not ${action} ${failedResults.length === 1 ? 'server' : 'servers'}: ${details}`,
      { timeout: 7000 },
    )
  } else if (servers.length > 1 || action === 'update') {
    notifySuccess(
      action === 'update'
        ? `Update started for ${servers.length === 1 ? servers[0]?.displayName : `${servers.length} servers`}.`
        : `${action[0]?.toUpperCase()}${action.slice(1)} requested for ${servers.length} servers.`,
      { timeout: 3500 },
    )
  }

  void getGameServers()
}

async function runServerAction(action: ServerAction, server: DisplayRow) {
  await runServerActions(action, [server])
}

async function runSelectedServerAction(action: ServerAction) {
  await runServerActions(action, selectedServersForAction.value[action], true)
}

const columns = ref([
  {
    name: 'name',
    label: 'Name',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.displayName,
    classes: 'server-name-cell',
    headerClasses: 'server-name-cell',
    sortable: true,
  },
  {
    name: 'status',
    label: 'Status',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.statusEnum,
    sortable: true,
  },
  {
    name: 'players',
    label: 'Players',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => getPlayerCounts(row).current ?? -1,
    sortable: true,
  },
  {
    name: 'resources',
    label: 'Resources',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => getResourceUsage(row).cpuPercent ?? -1,
    sortable: false,
  },
  {
    name: 'game',
    label: 'Game',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.gameName,
    sortable: true,
  },
  {
    name: 'node',
    label: 'Node',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.nodeName,
    sortable: true,
  },
  {
    name: 'version',
    label: 'Version',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => getDisplayVersion(row),
    classes: 'server-version-cell',
    sortable: false,
  },
  {
    name: 'owner',
    label: 'Owner',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.userName,
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
])
</script>

<style scoped>
.server-list-page--with-settings {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(390px, 480px);
  align-content: start;
  gap: 0 var(--xy-space-lg);
}

.server-list-page--with-settings > :deep(.server-list-header),
.server-list-page--with-settings > .server-list-error,
.server-list-page--with-settings > .server-list-notice {
  grid-column: 1 / -1;
}

.server-list-main {
  min-width: 0;
}

.server-list-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.server-list-summary__online {
  color: var(--xy-success-text-soft);
}

/* Bulk actions get their own full-width bar above the list, so they wrap
   instead of being squeezed into the title row. */
.server-selection-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm) var(--xy-space-md);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-sm);
}

.server-selection-bar__count {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  color: var(--xy-text-primary);
  white-space: nowrap;
}

.server-selection-bar__count .q-icon {
  color: var(--xy-accent);
}

.server-selection-bar__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-xs);
}

.server-selection-bar__delete {
  margin-left: var(--xy-space-sm);
}

.server-grid-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm) var(--xy-space-md);
  margin-bottom: var(--xy-space-sm);
  padding-inline: var(--xy-space-xs);
}

.server-grid-controls__sort {
  display: flex;
  align-items: center;
  gap: var(--xy-space-2xs);
}

.server-grid-controls__sort-select {
  min-width: 10rem;
}

.server-status-panel {
  scroll-margin-top: calc(var(--xy-header-stack-height) + var(--xy-space-md));
}

.server-list-error {
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

.server-list-notice {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  background: var(--xy-info-bg);
  border: 1px solid var(--xy-info-border);
  border-radius: var(--xy-radius-md);
}

.server-list-error > div {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.server-list-error span {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.server-player-count {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  white-space: nowrap;
}

.server-player-count .q-icon {
  color: var(--xy-accent);
}

.server-resource-usage {
  display: grid;
  gap: var(--xy-space-2xs);
  min-width: 9.5rem;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  line-height: 1.35;
}

.server-resource-usage span {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  white-space: nowrap;
}

.server-resource-usage .q-icon {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}

.server-table-actions,
.server-lifecycle-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-2xs);
  white-space: nowrap;
}

.server-table-actions > .q-separator {
  height: 1.75rem;
  margin-inline: var(--xy-space-xs);
}

/* The checkbox and Name stay pinned while the wide table scrolls sideways, so
   the row an action belongs to is always in view (actions pin right). */
.server-list-main :deep(.q-table .q-table--col-auto-width) {
  position: sticky;
  left: 0;
  z-index: 1;
  box-sizing: border-box;
  width: 4.5rem;
  min-width: 4.5rem;
  background-color: var(--xy-surface-0);
}

.server-list-main :deep(.q-table .server-name-cell) {
  position: sticky;
  left: 4.5rem;
  z-index: 1;
  background-color: var(--xy-surface-0);
  border-right: 1px solid var(--xy-border);
}

/* Long names wrap so the pinned column never takes over the scroll area. */
.server-list-main :deep(.q-table td.server-name-cell) {
  max-width: 16rem;
  white-space: normal;
  overflow-wrap: anywhere;
}

.server-list-main :deep(.q-table thead .q-table--col-auto-width),
.server-list-main :deep(.q-table thead .server-name-cell) {
  background-color: var(--xy-surface-2);
}

/* Long build strings wrap instead of pushing row actions off-screen. */
.server-list-main :deep(.server-version-cell) {
  min-width: 10rem;
  max-width: 14rem;
  white-space: normal;
}

.version-text {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  color: var(--xy-text-secondary);
}

.version-arrow {
  color: var(--xy-warning);
  margin: 0 0.25rem;
  font-size: var(--xy-font-size-xs);
}

.version-new {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  color: var(--xy-warning);
  font-weight: 600;
}

.version-na {
  color: var(--xy-text-muted);
  font-style: italic;
}

/* :deep because the slot root loses the scope id when the table switches to cards. */
.server-list-main :deep(.server-grid-item) {
  padding: var(--xy-space-xs);
}

.server-mobile-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.server-mobile-header {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
}

.server-mobile-select {
  margin-top: -0.25rem;
  margin-left: -0.5rem;
}

.server-mobile-identity {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.server-mobile-identity > span {
  overflow: hidden;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Same cyan Exo 2 name link as the table (.table-link), one step larger. */
.server-mobile-name {
  display: -webkit-box;
  font-size: var(--xy-font-size-base);
  line-height: 1.25;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

.server-mobile-details {
  display: grid;
  flex: 1;
  align-content: start;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
}

.server-mobile-detail-group {
  display: grid;
  gap: var(--xy-space-md);
}

/* Players and CPU take what they need, Memory the rest; the context row wraps
   to two columns in narrow cards. */
.server-mobile-health {
  grid-template-columns: auto auto minmax(0, 1fr);
}

.server-mobile-context {
  grid-template-columns: repeat(auto-fit, minmax(6.5rem, 1fr));
  padding-top: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.server-mobile-update {
  font-size: var(--xy-font-size-sm);
  overflow-wrap: anywhere;
}

.server-mobile-update .version-arrow {
  margin-left: 0;
}

.server-mobile-detail-group > div {
  display: grid;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.server-mobile-details strong {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.server-mobile-label {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.server-mobile-actions {
  flex-wrap: wrap;
  justify-content: space-between;
  gap: var(--xy-space-xs);
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

.q-item.server-card-menu-item {
  min-height: 44px;
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.q-item.server-card-menu-item--danger {
  color: var(--xy-danger-hover);
}

.server-mobile-row-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  margin-left: auto;
}

@media (max-width: 1023px) {
  .server-list-page--with-settings {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-lg);
  }
}

@media (max-width: 599px) {
  /* The search box takes its own row under the title and page buttons. */
  .server-list-header :deep(.xy-page-actions) {
    width: 100%;
    justify-content: flex-end;
  }

  .server-list-header :deep(.xy-search-input) {
    flex: 1 1 100%;
    order: 1;
  }

  /* Bulk actions become a bottom bar in thumb reach; the padding keeps the last card clear. */
  .server-list-page--selecting {
    padding-bottom: calc(var(--xy-space-md) + 7rem);
  }

  .server-selection-bar {
    position: fixed;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: var(--xy-z-sticky);
    margin: 0;
    padding: var(--xy-space-sm) var(--xy-space-md)
      max(var(--xy-space-sm), env(safe-area-inset-bottom));
    background: var(--xy-surface-1);
    border-width: 1px 0 0;
    border-radius: 0;
    box-shadow: var(--xy-shadow-sticky-lg);
  }

  .server-list-main :deep(.server-grid-item) {
    padding-inline: 0;
  }
}
</style>
