<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Game Servers</h1>
      <div class="xy-page-actions">
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="
            !lifecycleStateAuthoritative || loading || selectedGameServersForStart.length < 1
          "
          color="positive"
          label="Start selected"
          @click="startSelectedGameServers">
          <q-tooltip v-if="!lifecycleStateAuthoritative">
            Waiting for authoritative server status
          </q-tooltip>
        </q-btn>
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="
            !lifecycleStateAuthoritative || loading || selectedGameServersForStop.length < 1
          "
          color="warning"
          label="Stop selected"
          @click="stopSelectedGameServers">
          <q-tooltip v-if="!lifecycleStateAuthoritative">
            Waiting for authoritative server status
          </q-tooltip>
        </q-btn>
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="!lifecycleStateAuthoritative || loading"
          color="negative"
          label="Remove game server"
          @click="deleteGameServerAction(null)">
          <q-tooltip v-if="!lifecycleStateAuthoritative">
            Waiting for authoritative server status
          </q-tooltip>
        </q-btn>
        <q-input
          v-model="search"
          aria-label="Search game servers"
          class="xy-search-input"
          color="primary"
          debounce="300"
          dense
          outlined
          placeholder="Search...">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn
          v-if="showCreateButton"
          :disable="loading"
          color="primary"
          label="Create Game Server"
          to="/game-servers/create" />
      </div>
    </div>
    <div v-if="serverListError" class="server-list-error" role="alert" aria-live="assertive">
      <q-icon name="sync_problem" size="sm" />
      <div>
        <strong>Server status could not be refreshed.</strong>
        <span>{{ serverListError }} Lifecycle actions remain disabled.</span>
      </div>
      <q-btn :loading="loading" dense flat icon="refresh" label="Retry" @click="getGameServers" />
    </div>
    <div
      v-else-if="!lifecycleStateAuthoritative && !loading"
      class="server-list-notice"
      role="status">
      <q-icon name="sync" size="sm" />
      <span>Connecting to live server status. Lifecycle actions will be available shortly.</span>
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selectedGameServers"
        :columns="columns"
        :filter="search"
        :grid="$q.screen.lt.md"
        :loading="loading"
        :rows="displayRows"
        class="xy-standalone-table"
        flat
        hide-header-in-grid
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
                    class="server-mobile-name">
                    {{ props.row.displayName }}
                  </router-link>
                  <span>{{ props.row.gameName }}</span>
                </div>
                <status-badge :status="props.row.statusEnum" />
              </q-card-section>

              <q-separator />

              <q-card-section class="server-mobile-details">
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
                <q-btn
                  :to="`/game-servers/${props.row.id}/configuration`"
                  aria-label="Edit game server"
                  flat
                  icon="settings">
                  <q-tooltip>Edit game server</q-tooltip>
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
            <status-badge :status="props.row.statusEnum" style="margin-left: -1em"></status-badge>
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
          </q-td>
        </template>
        <template #body-cell-type="props">
          <q-td :props="props">
            <q-badge v-if="props.row.isLocal" color="positive" label="local" />
            <q-badge v-else class="badge-remote" label="remote" />
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="q-gutter-xs">
              <router-link :to="'/game-servers/' + props.row.id + '/configuration'">
                <q-btn
                  :icon="tabSettings"
                  aria-label="Edit game server"
                  class="text-main-brighter"
                  flat>
                  <q-tooltip>Edit game server</q-tooltip>
                </q-btn>
              </router-link>
              <span>
                <q-btn
                  :icon="tabTrash"
                  :disable="!lifecycleStateAuthoritative"
                  aria-label="Delete game server"
                  class="text-error-brighter"
                  flat
                  @click="deleteGameServerAction(props.row)">
                  <q-tooltip>
                    {{
                      lifecycleStateAuthoritative
                        ? 'Delete game server'
                        : 'Waiting for authoritative server status'
                    }}
                  </q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon class="q-mb-sm text-xy-muted" name="dns" size="3rem" />
            <div class="text-subtitle1">No game servers</div>
            <div class="text-caption text-xy-muted">Create a game server to get started.</div>
            <q-btn
              v-if="showCreateButton"
              class="q-mt-md"
              color="primary"
              label="Create Game Server"
              to="/game-servers/create" />
          </div>
        </template>
      </q-table>
    </div>
    <delete-game-server-dialog
      v-model:show-dialog="showDeleteGameServerDialog"
      :game-servers="selectedServersForDelete"
      @submit="deleteGameServerSubmitted"></delete-game-server-dialog>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { tabSettings, tabTrash } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { computed, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import DeleteGameServerDialog from '@/components/game_servers/DeleteGameServerDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import {
  Node,
  StartGameServerRequest,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequest,
  StopGameServerRequestSchema,
  type VersionInfo,
} from '@/proto/shared_pb'
import { useStorage } from '@vueuse/core'
import {
  AggregatedGameServer,
  ListAggregatedGameServersRequestSchema,
  ListNodesRequestSchema,
} from '@/proto/xylona_pb'
import {
  buildDisplayRows,
  type DisplayRow,
  extractRemoteNodeIDs,
  filterRowsByRemoteNodeIDs,
  sanitizeBootstrapCachedRows,
} from './server-list-cache'
import { getStartableServers, getStoppableServers } from './server-list-actions'
import { useUserAuthStore } from '@/stores/xylona'
import { resolveCanonicalVersionDisplay } from './version-display'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'

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
const selectedGameServers = ref([] as DisplayRow[])
const cachedDisplayRows = useStorage<DisplayRow[]>('game-server-display-rows-cache', [])
const cachedRemoteNodeIDs = useStorage<string[]>('game-server-remote-node-ids-cache', [])
const allowedRemoteNodeIDs = ref(new Set(cachedRemoteNodeIDs.value))
const $q = useQuasar()
let loadSequence = 0
let initialLoadComplete = false
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

const initialPagination = useStorage('game-server-pagination', {
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

const selectedServersForDelete = computed(() => {
  return selectedGameServers.value.map((s) => ({ id: s.id, name: s.displayName }))
})

const selectedGameServersForStart = computed(() => {
  return getStartableServers(selectedGameServers.value)
})

const selectedGameServersForStop = computed(() => {
  return getStoppableServers(selectedGameServers.value)
})

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
  await getGameServers()
  initialLoadComplete = true
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServerStatus', handleServerStatusUpdate)
  XylonaEventBus.off('gameServerVersion', handleServerVersionUpdate)
  XylonaEventBus.off('websocketConnected', handleWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', handleWebsocketDisconnect)
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
}

function handleWebsocketReconnect() {
  if (!initialLoadComplete || loading.value) {
    return
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

  for (const row of cachedDisplayRows.value) {
    if (row.id === serverID) {
      row.statusEnum = serverStatus
    }
  }

  updateLiveServerData((server) => {
    if (server.isLocal && server.localServer && server.localServer.id === serverID) {
      server.localServer.status = serverStatus
      return true
    }
    if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
      server.remoteServer.status = serverStatus
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

async function startSelectedGameServers() {
  if (
    !lifecycleStateAuthoritative.value ||
    selectedGameServersForStart.value.length < 1 ||
    loading.value
  ) {
    return
  }

  loading.value = true
  const failedServerNames: string[] = []

  try {
    for (const selectedServer of selectedGameServersForStart.value) {
      const request: StartGameServerRequest = create(StartGameServerRequestSchema, {})
      request.serverId = selectedServer.id
      try {
        await GetXylonaClient().startGameServer(request)
      } catch (errStart) {
        failedServerNames.push(selectedServer.displayName)
        console.error(errStart)
      }
    }
  } finally {
    loading.value = false
  }

  selectedGameServers.value = []

  if (failedServerNames.length > 0) {
    $q.notify({
      caption: `Failed to start: ${failedServerNames.join(', ')}`,
      type: 'xylona-error',
      position: 'top-right',
      timeout: 5000,
    })
  }

  void getGameServers()
}

async function stopSelectedGameServers() {
  if (
    !lifecycleStateAuthoritative.value ||
    selectedGameServersForStop.value.length < 1 ||
    loading.value
  ) {
    return
  }

  loading.value = true
  const failedServerNames: string[] = []

  try {
    for (const selectedServer of selectedGameServersForStop.value) {
      const request: StopGameServerRequest = create(StopGameServerRequestSchema, {})
      request.serverId = selectedServer.id
      try {
        await GetXylonaClient().stopGameServer(request)
      } catch (errStop) {
        failedServerNames.push(selectedServer.displayName)
        console.error(errStop)
      }
    }
  } finally {
    loading.value = false
  }

  selectedGameServers.value = []

  if (failedServerNames.length > 0) {
    $q.notify({
      caption: `Failed to stop: ${failedServerNames.join(', ')}`,
      type: 'xylona-error',
      position: 'top-right',
      timeout: 5000,
    })
  }

  void getGameServers()
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
    name: 'version',
    label: 'Version',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => getDisplayVersion(row),
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
    name: 'owner',
    label: 'Owner',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.userName,
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
    name: 'node',
    label: 'Node',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => row.nodeName,
    sortable: true,
  },
  {
    name: 'type',
    label: 'Type',
    required: true,
    align: 'left' as const,
    field: (row: DisplayRow) => (row.isLocal ? 'local' : 'remote'),
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
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.server-mobile-details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
}

.server-mobile-details > div {
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
  min-height: 3.5rem;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-3);
}

@media (max-width: 599px) {
  .server-grid-item {
    padding-inline: 0;
  }

  .server-mobile-details {
    gap: var(--xy-space-sm) var(--xy-space-md);
  }
}
</style>
