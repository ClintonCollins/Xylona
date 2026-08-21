<template>
  <q-page
    class="server-list-page xy-page-content"
    :class="{ 'server-list-page--with-settings': showStatusPageSettings }">
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
        <div class="server-selection-region">
          <q-toolbar
            v-if="selectedGameServers.length > 0"
            aria-label="Selected game server actions"
            class="server-selection-toolbar">
            <div class="server-selection-toolbar__count">
              <q-icon name="checklist" size="sm" />
              <strong aria-hidden="true">
                {{ selectedGameServers.length
                }}<span class="server-selection-toolbar__count-label"> selected</span>
              </strong>
              <span class="xy-visually-hidden">{{ selectedGameServers.length }} selected</span>
            </div>
            <q-separator class="server-selection-toolbar__separator" vertical />
            <q-btn
              :aria-label="`Start ${selectedGameServersForStart.length} selected game servers`"
              :disable="
                !lifecycleStateAuthoritative || loading || selectedGameServersForStart.length < 1
              "
              color="positive"
              dense
              icon="play_arrow"
              :label="`Start ${selectedGameServersForStart.length}`"
              no-caps
              outline
              @click="startSelectedGameServers">
              <q-tooltip>Start selected game servers</q-tooltip>
            </q-btn>
            <q-btn
              :aria-label="`Restart ${selectedGameServersForRestart.length} selected game servers`"
              :disable="
                !lifecycleStateAuthoritative || loading || selectedGameServersForRestart.length < 1
              "
              color="warning"
              dense
              icon="restart_alt"
              :label="`Restart ${selectedGameServersForRestart.length}`"
              no-caps
              outline
              @click="restartSelectedGameServers">
              <q-tooltip>Restart selected game servers</q-tooltip>
            </q-btn>
            <q-btn
              :aria-label="`Stop ${selectedGameServersForStop.length} selected game servers`"
              :disable="
                !lifecycleStateAuthoritative || loading || selectedGameServersForStop.length < 1
              "
              color="negative"
              dense
              icon="stop"
              :label="`Stop ${selectedGameServersForStop.length}`"
              no-caps
              outline
              @click="stopSelectedGameServers">
              <q-tooltip>Stop selected game servers</q-tooltip>
            </q-btn>
            <q-btn
              :aria-label="`Update ${selectedGameServersForUpdate.length} selected game servers`"
              :disable="
                !lifecycleStateAuthoritative || loading || selectedGameServersForUpdate.length < 1
              "
              color="accent"
              dense
              icon="system_update_alt"
              :label="`Update ${selectedGameServersForUpdate.length}`"
              no-caps
              outline
              @click="updateSelectedGameServers">
              <q-tooltip>Update selected game servers</q-tooltip>
            </q-btn>
            <q-separator class="server-selection-toolbar__separator" vertical />
            <q-btn
              :aria-label="`Remove ${selectedGameServers.length} selected game servers`"
              :disable="!lifecycleStateAuthoritative || loading"
              color="negative"
              dense
              flat
              icon="delete_outline"
              label="Remove"
              no-caps
              @click="deleteGameServerAction(null)">
              <q-tooltip>Remove selected game servers</q-tooltip>
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
          </q-toolbar>
        </div>
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
          :color="showStatusPageSettings ? 'primary' : undefined"
          flat
          icon="public"
          label="Public status page"
          no-caps
          @click="showStatusPageSettings = !showStatusPageSettings" />
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
      v-else-if="!lifecycleStateAuthoritative && !loading"
      class="server-list-notice"
      role="status">
      <q-icon name="sync" size="sm" />
      <span>
        Connecting to live server status. Server controls are unavailable until the connection is
        ready.
      </span>
    </div>
    <div class="server-list-main">
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selectedGameServers"
        :columns="columns"
        :filter="search"
        :grid="$q.screen.lt.lg"
        :loading="loading"
        :rows="displayRows"
        class="xy-standalone-table"
        flat
        hide-header-in-grid
        hide-selected-banner
        row-key="compositeId"
        selection="multiple">
        <template #item="props">
          <div class="server-grid-item col-12">
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
                    class="server-mobile-name">
                    {{ props.row.displayName }}
                  </router-link>
                  <span>{{ props.row.gameName }}</span>
                </div>
                <status-badge :status="props.row.statusEnum" />
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
                  <div class="server-mobile-memory">
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
                  flat
                  icon="terminal"
                  label="Console"
                  no-caps />
                <q-space />
                <div class="server-lifecycle-actions">
                  <q-btn
                    :aria-label="`Start ${props.row.displayName}`"
                    :disable="!canRunServerAction(props.row, 'start')"
                    :loading="isServerActionPending(props.row, 'start')"
                    color="positive"
                    dense
                    flat
                    icon="play_arrow"
                    round
                    @click="runServerAction('start', props.row)">
                    <q-tooltip>{{ getServerActionTooltip(props.row, 'start') }}</q-tooltip>
                  </q-btn>
                  <q-btn
                    :aria-label="`Restart ${props.row.displayName}`"
                    :disable="!canRunServerAction(props.row, 'restart')"
                    :loading="isServerActionPending(props.row, 'restart')"
                    color="warning"
                    dense
                    flat
                    icon="restart_alt"
                    round
                    @click="runServerAction('restart', props.row)">
                    <q-tooltip>{{ getServerActionTooltip(props.row, 'restart') }}</q-tooltip>
                  </q-btn>
                  <q-btn
                    :aria-label="`Stop ${props.row.displayName}`"
                    :disable="!canRunServerAction(props.row, 'stop')"
                    :loading="isServerActionPending(props.row, 'stop')"
                    color="negative"
                    dense
                    flat
                    icon="stop"
                    round
                    @click="runServerAction('stop', props.row)">
                    <q-tooltip>{{ getServerActionTooltip(props.row, 'stop') }}</q-tooltip>
                  </q-btn>
                  <q-btn
                    :aria-label="`Update ${props.row.displayName}`"
                    :disable="!canRunServerAction(props.row, 'update')"
                    :loading="isServerActionPending(props.row, 'update')"
                    color="accent"
                    dense
                    flat
                    icon="system_update_alt"
                    round
                    @click="runServerAction('update', props.row)">
                    <q-tooltip>{{ getServerActionTooltip(props.row, 'update') }}</q-tooltip>
                  </q-btn>
                </div>
                <q-btn
                  :to="`/game-servers/${props.row.id}/configuration`"
                  :aria-label="`Configure ${props.row.displayName}`"
                  flat
                  icon="settings">
                  <q-tooltip>Configure {{ props.row.displayName }}</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Delete ${props.row.displayName}`"
                  :disable="!lifecycleStateAuthoritative"
                  class="text-error-brighter"
                  flat
                  icon="delete"
                  @click="deleteGameServerAction(props.row)">
                  <q-tooltip>
                    {{
                      lifecycleStateAuthoritative
                        ? `Delete ${props.row.displayName}`
                        : 'Waiting for authoritative server status'
                    }}
                  </q-tooltip>
                </q-btn>
              </q-card-actions>
            </q-card>
          </div>
        </template>
        <template #body-cell-name="props">
          <q-td :props="props">
            <router-link :to="'/game-servers/' + props.row.id + '/console'" class="table-link">
              {{ props.row.displayName }}
            </router-link>
            <q-badge v-if="props.row.isStale" class="q-ml-xs" color="warning" label="stale" />
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <status-badge :status="props.row.statusEnum"></status-badge>
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
              :class="{ 'badge-remote': !props.row.isLocal }"
              class="q-ml-xs"
              :color="props.row.isLocal ? 'positive' : undefined"
              :label="props.row.isLocal ? 'local' : 'remote'" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="server-table-actions">
              <div class="server-lifecycle-actions">
                <q-btn
                  :aria-label="`Start ${props.row.displayName}`"
                  :disable="!canRunServerAction(props.row, 'start')"
                  :loading="isServerActionPending(props.row, 'start')"
                  color="positive"
                  dense
                  flat
                  icon="play_arrow"
                  round
                  @click="runServerAction('start', props.row)">
                  <q-tooltip>{{ getServerActionTooltip(props.row, 'start') }}</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Restart ${props.row.displayName}`"
                  :disable="!canRunServerAction(props.row, 'restart')"
                  :loading="isServerActionPending(props.row, 'restart')"
                  color="warning"
                  dense
                  flat
                  icon="restart_alt"
                  round
                  @click="runServerAction('restart', props.row)">
                  <q-tooltip>{{ getServerActionTooltip(props.row, 'restart') }}</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Stop ${props.row.displayName}`"
                  :disable="!canRunServerAction(props.row, 'stop')"
                  :loading="isServerActionPending(props.row, 'stop')"
                  color="negative"
                  dense
                  flat
                  icon="stop"
                  round
                  @click="runServerAction('stop', props.row)">
                  <q-tooltip>{{ getServerActionTooltip(props.row, 'stop') }}</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Update ${props.row.displayName}`"
                  :disable="!canRunServerAction(props.row, 'update')"
                  :loading="isServerActionPending(props.row, 'update')"
                  color="accent"
                  dense
                  flat
                  icon="system_update_alt"
                  round
                  @click="runServerAction('update', props.row)">
                  <q-tooltip>{{ getServerActionTooltip(props.row, 'update') }}</q-tooltip>
                </q-btn>
              </div>
              <q-separator vertical />
              <q-btn
                :to="'/game-servers/' + props.row.id + '/configuration'"
                :icon="tabSettings"
                :aria-label="`Configure ${props.row.displayName}`"
                class="text-main-brighter"
                dense
                flat
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
                  @click="deleteGameServerAction(props.row)">
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
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon
              class="q-mb-sm text-xy-muted"
              :name="search.trim().length > 0 ? 'search_off' : 'dns'"
              size="3rem" />
            <div class="text-subtitle1">
              {{ search.trim().length > 0 ? 'No matching game servers' : 'No game servers' }}
            </div>
            <div class="server-empty-state-copy text-caption text-xy-muted">
              <template v-if="search.trim().length > 0">
                No game servers match “{{ search.trim() }}”.
              </template>
              <template v-else>Create a game server to get started.</template>
            </div>
            <q-btn
              v-if="search.trim().length > 0"
              class="q-mt-md"
              flat
              label="Clear search"
              @click="search = ''" />
            <q-btn
              v-else-if="showCreateButton"
              class="q-mt-md"
              color="primary"
              label="Create Game Server"
              to="/game-servers/create" />
          </div>
        </template>
      </q-table>
    </div>
    <game-server-status-page-settings-panel
      v-if="showStatusPageSettings"
      @close="showStatusPageSettings = false" />
    <delete-game-server-dialog
      v-model:show-dialog="showDeleteGameServerDialog"
      :game-servers="selectedServersForDelete"
      @submit="deleteGameServerSubmitted"></delete-game-server-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create, toJsonString } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import {
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import DeleteGameServerDialog from '@/components/game_servers/DeleteGameServerDialog.vue'
import GameServerStatusPageSettingsPanel from '@/components/game_servers/GameServerStatusPageSettingsPanel.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import type { StepState } from '@/components/game_servers/UpdateProgressPanel.types'
import StatusBadge from '@/components/StatusBadge.vue'
import {
  type AllServersQueryInfo,
  Node,
  ServerQuery_Type,
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
import { type AllServersMetrics, Request, Request_Type, RequestSchema } from '@/proto/websocket_pb'
import {
  buildDisplayRows,
  type DisplayRow,
  extractRemoteNodeIDs,
  filterRowsByRemoteNodeIDs,
  sanitizeBootstrapCachedRows,
} from './server-list-cache'
import {
  buildLifecycleConfirmation,
  canRestartServer,
  canStartServer,
  canStopServer,
  canUpdateServer,
  getRestartableServers,
  getStartableServers,
  getStoppableServers,
  getUpdateableServers,
  type LifecycleConfirmAction,
} from './server-list-actions'
import { useUserAuthStore } from '@/stores/xylona'
import { resolveCanonicalVersionDisplay } from './version-display'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'
import { formatMetricBytes } from './metrics-format'
import { recordLifecycleIntent } from '@/utils/game-server-notifications'
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
const selectedGameServers = ref([] as DisplayRow[])
type ServerAction = 'start' | 'stop' | 'restart' | 'update'
type ServerPlayerCounts = { current: number; max: number }
type ServerResourceUsage = {
  cpuPercent: number | null
  memoryBytes: number | null
  memoryPercent: number | null
}
const pendingActionByServerID = ref(new Map<string, ServerAction>())
const updateStepsByServerID = new Map<string, StepState[]>()
const playerCountsByServerID = ref(new Map<string, ServerPlayerCounts>())
const resourceUsageByServerID = ref(new Map<string, ServerResourceUsage>())
const cachedDisplayRows = usePersistedRef<DisplayRow[]>('game-server-display-rows-cache', [])
const cachedRemoteNodeIDs = usePersistedRef<string[]>('game-server-remote-node-ids-cache', [])
const allowedRemoteNodeIDs = ref(new Set(cachedRemoteNodeIDs.value))
const $q = useQuasar()
let loadSequence = 0
let initialLoadComplete = false
let reconnectRefreshQueued = false
let serverListUnmounted = false
const subscribedMetricsServerIDs = new Set<string>()
type BufferedLiveServerState = {
  status?: Status
  version?: string
  versionInfo?: VersionInfo
}
const bufferedLiveServerStateByID = new Map<string, BufferedLiveServerState>()

const sanitizedCachedDisplayRows = sanitizeBootstrapCachedRows(cachedDisplayRows.value)
if (sanitizedCachedDisplayRows.some((row, index) => row !== cachedDisplayRows.value[index])) {
  cachedDisplayRows.value = sanitizedCachedDisplayRows
}

const initialPagination = usePersistedRef('game-server-pagination', {
  rowsPerPage: 25,
  page: 1,
})
const authStore = useUserAuthStore()
const showCreateButton = computed(() => authStore.user?.superUser ?? false)
const hasFetchedLiveRows = computed(() => {
  return aggregatedServers.value !== null
})

const liveDisplayRows = computed((): DisplayRow[] => {
  return buildDisplayRows(aggregatedServers.value ?? [], nodesByID.value)
})

const displayRows = computed((): DisplayRow[] => {
  if (hasFetchedLiveRows.value) {
    return liveDisplayRows.value
  }
  return filterRowsByRemoteNodeIDs(cachedDisplayRows.value, allowedRemoteNodeIDs.value).filter(
    (row) => !row.isLocal || row.statusEnum !== Status.ONLINE,
  )
})

const onlineServerCount = computed(
  () => displayRows.value.filter((server) => server.statusEnum === Status.ONLINE).length,
)

const totalPlayerCounts = computed(() => {
  return displayRows.value.reduce(
    (total, server) => {
      const counts = getPlayerCounts(server)
      total.current += counts.current
      total.max += counts.max
      return total
    },
    { current: 0, max: 0 },
  )
})

const selectedServersForDelete = computed(() => {
  return selectedGameServers.value.map((s) => ({ id: s.id, name: s.displayName }))
})

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
      return canStopServer(server.statusEnum) && hasPermission(server, 'game_server.stop')
    case 'restart':
      return canRestartServer(server.statusEnum) && hasPermission(server, 'game_server.restart')
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
  if ((action === 'stop' || action === 'restart') && server.statusEnum !== Status.ONLINE) {
    return `${action === 'stop' ? 'Stop' : 'Restart'} is available when the server is online`
  }
  if (
    action === 'update' &&
    server.statusEnum !== Status.ONLINE &&
    server.statusEnum !== Status.OFFLINE
  ) {
    return 'Update is unavailable while another operation is running'
  }

  return `${action[0]?.toUpperCase()}${action.slice(1)} ${server.displayName}`
}

function getPlayerCounts(server: DisplayRow): ServerPlayerCounts {
  const liveCounts = lifecycleStateAuthoritative.value
    ? playerCountsByServerID.value.get(server.id)
    : undefined
  return {
    current:
      server.statusEnum === Status.ONLINE && lifecycleStateAuthoritative.value
        ? (liveCounts?.current ?? server.currentPlayers ?? 0)
        : 0,
    max: liveCounts?.max || server.maxPlayers || 0,
  }
}

function getPlayerCountLabel(server: DisplayRow): string {
  const counts = getPlayerCounts(server)
  return counts.max > 0 ? `${counts.current} / ${counts.max}` : `${counts.current}`
}

function getResourceUsage(server: DisplayRow): ServerResourceUsage {
  if (
    !lifecycleStateAuthoritative.value ||
    !hasPermission(server, 'game_server.metrics') ||
    server.statusEnum !== Status.ONLINE
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
    const server = liveDisplayRows.value.find((row) => row.id === serverID)
    if (!server || server.statusEnum !== Status.ONLINE) {
      nextCounts.delete(serverID)
      continue
    }
    switch (serverQuery.type) {
      case ServerQuery_Type.Minecraft:
        if (serverQuery.minecraft) {
          nextCounts.set(serverID, {
            current: serverQuery.minecraft.numberOfPlayers,
            max: serverQuery.minecraft.maxPlayers,
          })
        }
        break
      case ServerQuery_Type.Source:
        if (serverQuery.source) {
          nextCounts.set(serverID, {
            current: serverQuery.source.players,
            max: serverQuery.source.maxPlayers,
          })
        }
        break
      case ServerQuery_Type.Palworld:
        if (serverQuery.palworld) {
          nextCounts.set(serverID, {
            current: serverQuery.palworld.players,
            max: serverQuery.palworld.maxPlayers,
          })
        }
        break
    }
  }
  playerCountsByServerID.value = nextCounts
}

function applyServerMetrics(metrics: AllServersMetrics) {
  const nextUsage = new Map(resourceUsageByServerID.value)
  for (const [serverID, serverMetrics] of Object.entries(metrics.servers)) {
    const server = liveDisplayRows.value.find((row) => row.id === serverID)
    if (!server || server.statusEnum !== Status.ONLINE) {
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

function sendMetricsSubscription(serverID: string, type: Request_Type): boolean {
  const websocket = GetOrCreateXylonaWebsocketClient()
  if (!websocket.isOpen()) {
    return false
  }

  const request: Request = create(RequestSchema, {
    gameServerId: serverID,
    type,
  })
  websocket.send(toJsonString(RequestSchema, request))
  return true
}

function syncMetricsSubscriptions(serverIDs: string[]) {
  const desiredServerIDs = new Set(serverIDs)
  for (const serverID of subscribedMetricsServerIDs) {
    if (desiredServerIDs.has(serverID)) {
      continue
    }
    try {
      sendMetricsSubscription(serverID, Request_Type.UnsubscribeServerMetrics)
    } catch (error) {
      console.error('Failed to unsubscribe from server metrics', error)
    }
    subscribedMetricsServerIDs.delete(serverID)
  }

  for (const serverID of desiredServerIDs) {
    if (subscribedMetricsServerIDs.has(serverID)) {
      continue
    }
    try {
      if (sendMetricsSubscription(serverID, Request_Type.SubscribeServerMetrics)) {
        subscribedMetricsServerIDs.add(serverID)
      }
    } catch (error) {
      console.error('Failed to subscribe to server metrics', error)
    }
  }
}

function clearMetricsSubscriptions() {
  for (const serverID of subscribedMetricsServerIDs) {
    try {
      sendMetricsSubscription(serverID, Request_Type.UnsubscribeServerMetrics)
    } catch (error) {
      console.error('Failed to unsubscribe from server metrics', error)
    }
  }
  subscribedMetricsServerIDs.clear()
}

function applyNodesResponse(nodes: Node[]) {
  nodesByID.value = new Map(nodes.map((node) => [node.id, node]))
  const remoteNodeIDs = extractRemoteNodeIDs(nodes)
  cachedRemoteNodeIDs.value = [...remoteNodeIDs]
  allowedRemoteNodeIDs.value = remoteNodeIDs
  if (aggregatedServers.value !== null) {
    cacheAggregatedRows(aggregatedServers.value)
    return
  }
  cachedDisplayRows.value = filterRowsByRemoteNodeIDs(cachedDisplayRows.value, remoteNodeIDs)
}

function cacheAggregatedRows(servers: AggregatedGameServer[]) {
  cachedDisplayRows.value = filterRowsByRemoteNodeIDs(
    buildDisplayRows(servers, nodesByID.value).map((row) => ({
      ...row,
      versionInfo: undefined,
    })),
    allowedRemoteNodeIDs.value,
  )
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
  clearMetricsSubscriptions()
})

async function getGameServers() {
  const loadID = ++loadSequence
  serverStatusSnapshotFresh.value = false
  aggregatedServers.value = null
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
      cacheAggregatedRows(servers)
      syncMetricsSubscriptions(
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
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
        caption: 'Failed to load game servers: ' + ConnectErrorToString(ConnectError.from(reason)),
        icon: 'report_problem',
      })
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
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
        caption: 'Failed to load nodes: ' + ConnectErrorToString(ConnectError.from(reason)),
        icon: 'report_problem',
      })
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
  subscribedMetricsServerIDs.clear()
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

async function deleteGameServerAction(row: DisplayRow | null) {
  if (!lifecycleStateAuthoritative.value) {
    return
  }
  if (row !== null) {
    selectedGameServers.value = [row]
  }
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
  const failedIDs = new Set(result.failed.map((failure) => failure.id))
  selectedGameServers.value = displayRows.value.filter((row) => failedIDs.has(row.id))
}

function setServerStatus(serverID: string, serverStatus: Status) {
  recordBufferedLiveServerState(serverID, { status: serverStatus })

  if (serverStatus !== Status.ONLINE) {
    const nextPlayerCounts = new Map(playerCountsByServerID.value)
    nextPlayerCounts.delete(serverID)
    playerCountsByServerID.value = nextPlayerCounts

    const nextResourceUsage = new Map(resourceUsageByServerID.value)
    nextResourceUsage.delete(serverID)
    resourceUsageByServerID.value = nextResourceUsage
  }

  for (const row of cachedDisplayRows.value) {
    if (row.id === serverID) {
      row.statusEnum = serverStatus
      if (serverStatus !== Status.ONLINE) {
        row.currentPlayers = 0
        row.cpuPercent = null
        row.memoryBytes = null
        row.memoryPercent = null
      }
    }
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

  for (const row of cachedDisplayRows.value) {
    if (row.id === serverID) {
      row.version = version
    }
  }

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
  const runningCount = servers.filter((server) => server.statusEnum === Status.ONLINE).length
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

async function confirmLifecycleServers(
  action: LifecycleConfirmAction,
  servers: DisplayRow[],
): Promise<boolean> {
  const confirmation = buildLifecycleConfirmation(
    action,
    servers.map((server) => ({
      displayName: server.displayName,
      playerCount: getPlayerCounts(server).current,
    })),
  )
  if (confirmation === null) {
    return true
  }

  return new Promise((resolve) => {
    let settled = false
    $q.dialog({
      title: confirmation.title,
      message: confirmation.message,
      cancel: true,
      persistent: true,
      ok: {
        label: confirmation.confirmLabel,
        color: confirmation.confirmColor,
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
    $q.notify({
      caption: `Could not ${action} ${failedResults.length === 1 ? 'server' : 'servers'}: ${details}`,
      type: 'xylona-error',
      position: 'top-right',
      timeout: 7000,
    })
  } else if (servers.length > 1 || action === 'update') {
    $q.notify({
      caption:
        action === 'update'
          ? `Update started for ${servers.length === 1 ? servers[0]?.displayName : `${servers.length} servers`}.`
          : `${action[0]?.toUpperCase()}${action.slice(1)} requested for ${servers.length} servers.`,
      type: 'positive',
      position: 'top-right',
      timeout: 3500,
    })
  }

  void getGameServers()
}

async function runServerAction(action: ServerAction, server: DisplayRow) {
  await runServerActions(action, [server])
}

async function startSelectedGameServers() {
  await runServerActions('start', selectedGameServersForStart.value, true)
}

async function stopSelectedGameServers() {
  await runServerActions('stop', selectedGameServersForStop.value, true)
}

async function restartSelectedGameServers() {
  await runServerActions('restart', selectedGameServersForRestart.value, true)
}

async function updateSelectedGameServers() {
  await runServerActions('update', selectedGameServersForUpdate.value, true)
}

const columns = ref([
  {
    name: 'name',
    label: 'Name',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.displayName,
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
    field: (row: DisplayRow) => getPlayerCounts(row).current,
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

.server-list-header {
  position: relative;
  padding-bottom: calc(var(--xy-toolbar-height) + var(--xy-space-sm));
}

.server-list-header :deep(.xy-page-actions) {
  min-width: 0;
  min-height: var(--xy-toolbar-height);
}

.server-selection-region {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  display: flex;
  justify-content: flex-end;
  min-height: var(--xy-toolbar-height);
}

.server-selection-toolbar {
  width: auto;
  max-width: 100%;
  min-height: var(--xy-toolbar-height);
  gap: var(--xy-space-xs);
  padding-inline: var(--xy-space-sm);
  overflow-x: auto;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-sm);
}

.server-selection-toolbar__count {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  color: var(--xy-text-primary);
  white-space: nowrap;
}

.server-selection-toolbar__count .q-icon {
  color: var(--xy-accent);
}

.server-selection-toolbar__separator {
  height: 1.75rem;
  margin-inline: var(--xy-space-xs);
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

.badge-remote {
  background-color: var(--xy-surface-4);
  color: var(--xy-text-secondary);
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
  font-size: 0.9rem;
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

.version-text {
  font-family: var(--xy-font-mono);
  font-size: 0.8rem;
  color: var(--xy-text-secondary);
}

.version-arrow {
  color: var(--xy-warning);
  margin: 0 0.25rem;
  font-size: 0.75rem;
}

.version-new {
  font-family: var(--xy-font-mono);
  font-size: 0.8rem;
  color: var(--xy-warning);
  font-weight: 600;
}

.version-na {
  color: var(--xy-text-muted);
  font-style: italic;
}

.server-grid-item {
  padding: var(--xy-space-xs);
}

.server-mobile-card {
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

.server-mobile-name {
  display: -webkit-box;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  line-height: 1.25;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

.server-mobile-details {
  display: grid;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
}

.server-mobile-detail-group {
  display: grid;
  gap: var(--xy-space-md);
}

.server-mobile-health {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.server-mobile-context {
  grid-template-columns: repeat(4, minmax(0, 1fr));
  padding-top: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
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

.server-empty-state-copy {
  max-width: 100%;
  overflow-wrap: anywhere;
  text-align: center;
}

.server-mobile-actions {
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

@media (max-width: 1919px) {
  .server-selection-toolbar :deep(.q-btn__content > .block) {
    display: none !important;
  }

  .server-selection-toolbar :deep(.q-icon.on-left) {
    margin-right: 0;
  }
}

@media (max-width: 1023px) {
  .server-list-page--with-settings {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-lg);
  }
}

@media (max-width: 599px) {
  .server-list-header :deep(.xy-page-actions) {
    width: 100%;
    min-height: var(--xy-toolbar-height);
    flex-wrap: nowrap;
  }

  .server-selection-toolbar {
    gap: var(--xy-space-2xs);
    padding-inline: var(--xy-space-xs);
  }

  .server-selection-toolbar__separator {
    display: none;
  }

  .server-selection-toolbar__count-label {
    display: none;
  }

  .server-grid-item {
    padding-inline: 0;
  }

  .server-mobile-health,
  .server-mobile-context {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--xy-space-sm) var(--xy-space-md);
  }

  .server-mobile-memory {
    grid-column: 1 / -1;
  }

  .server-mobile-actions > .q-space {
    display: none;
  }

  .server-lifecycle-actions {
    margin-left: auto;
  }
}
</style>
