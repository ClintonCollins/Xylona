<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Game Servers</h1>
      <div class="xy-page-actions">
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="loading || selectedGameServersForStart.length < 1"
          color="positive"
          label="Start selected"
          @click="startSelectedGameServers" />
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="loading || selectedGameServersForStop.length < 1"
          color="warning"
          label="Stop selected"
          @click="stopSelectedGameServers" />
        <q-btn
          v-if="selectedGameServers.length >= 1"
          :disable="loading"
          color="negative"
          label="Remove game server"
          @click="deleteGameServerAction(null)" />
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
                  aria-label="Delete game server"
                  class="text-error-brighter"
                  flat
                  @click="deleteGameServerAction(props.row)">
                  <q-tooltip>Delete game server</q-tooltip>
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
  GameServer,
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
  AggregatedGameServerSchema,
  ListAggregatedGameServersRequestSchema,
  ListGameServersRequestSchema,
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

const bootstrapServers = ref<AggregatedGameServer[] | null>(null)
const aggregatedServers = ref<AggregatedGameServer[] | null>(null)
const nodesByID = ref(new Map<string, Node>())
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const selectedGameServers = ref([] as DisplayRow[])
const cachedDisplayRows = useStorage<DisplayRow[]>('game-server-display-rows-cache', [])
const cachedRemoteNodeIDs = useStorage<string[]>('game-server-remote-node-ids-cache', [])
const allowedRemoteNodeIDs = ref(new Set(cachedRemoteNodeIDs.value))
const $q = useQuasar()
let loadSequence = 0
const dirtyLiveServerCompositeIDs = new Set<string>()

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
const preferredServers = computed((): AggregatedGameServer[] => {
  if (aggregatedServers.value !== null) {
    return aggregatedServers.value
  }
  if (bootstrapServers.value !== null) {
    return bootstrapServers.value
  }
  return []
})
const hasFetchedLiveRows = computed(() => {
  return aggregatedServers.value !== null || bootstrapServers.value !== null
})

const liveDisplayRows = computed((): DisplayRow[] => {
  return buildDisplayRows(preferredServers.value, nodesByID.value)
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

function buildLocalAggregatedServers(gameServers: GameServer[]): AggregatedGameServer[] {
  return gameServers.map((gameServer) =>
    create(AggregatedGameServerSchema, {
      isLocal: true,
      localServer: gameServer,
    }),
  )
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

function getAggregatedServerCompositeID(server: AggregatedGameServer): string | null {
  if (server.isLocal && server.localServer) {
    return 'local/' + server.localServer.id
  }
  if (server.isLocal || !server.remoteServer) {
    return null
  }

  const sourceNodeID = server.remoteServer.sourceNodeId || server.remoteServer.nodeId
  return sourceNodeID + '/' + server.remoteServer.remoteServerId
}

function mergeLiveServerState(servers: AggregatedGameServer[]): AggregatedGameServer[] {
  const currentStateByCompositeID = new Map<
    string,
    { status: Status; version: string; versionInfo?: VersionInfo }
  >()

  for (const liveServers of [bootstrapServers.value, aggregatedServers.value]) {
    if (liveServers === null) {
      continue
    }

    for (const liveServer of liveServers) {
      const compositeID = getAggregatedServerCompositeID(liveServer)
      if (compositeID === null) {
        continue
      }

      if (liveServer.isLocal && liveServer.localServer) {
        currentStateByCompositeID.set(compositeID, {
          status: liveServer.localServer.status,
          version: liveServer.localServer.version,
          versionInfo: liveServer.localServer.versionInfo,
        })
        continue
      }
      if (!liveServer.isLocal && liveServer.remoteServer) {
        currentStateByCompositeID.set(compositeID, {
          status: liveServer.remoteServer.status,
          version: liveServer.remoteServer.version,
          versionInfo: liveServer.remoteServer.versionInfo,
        })
      }
    }
  }

  for (const server of servers) {
    const compositeID = getAggregatedServerCompositeID(server)
    if (compositeID === null) {
      continue
    }
    if (!dirtyLiveServerCompositeIDs.has(compositeID)) {
      continue
    }

    const currentState = currentStateByCompositeID.get(compositeID)
    if (!currentState) {
      continue
    }

    if (server.isLocal && server.localServer) {
      server.localServer.status = currentState.status
      server.localServer.version = currentState.version
      server.localServer.versionInfo = currentState.versionInfo
      continue
    }
    if (!server.isLocal && server.remoteServer) {
      server.remoteServer.status = currentState.status
      server.remoteServer.version = currentState.version
      server.remoteServer.versionInfo = currentState.versionInfo
    }
  }

  return servers
}

onMounted(async () => {
  await getGameServers()
  watchServerStatusChanges()
  watchServerVersionChanges()
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServerStatus', handleServerStatusUpdate)
  XylonaEventBus.off('gameServerVersion', handleServerVersionUpdate)
})

async function getGameServers() {
  const loadID = ++loadSequence
  bootstrapServers.value = null
  aggregatedServers.value = null
  dirtyLiveServerCompositeIDs.clear()
  loading.value = true
  let didRenderRows = false
  const xylonaClient = GetXylonaClient()

  const finishInitialRender = () => {
    if (loadID !== loadSequence) {
      return
    }
    if (didRenderRows) {
      return
    }

    didRenderRows = true
    loading.value = false
  }

  const aggregatedRequest = xylonaClient
    .listAggregatedGameServers(create(ListAggregatedGameServersRequestSchema, {}))
    .then((response) => {
      if (loadID !== loadSequence) {
        return
      }

      const mergedServers = mergeLiveServerState(response.servers)
      aggregatedServers.value = mergedServers
      cacheAggregatedRows(mergedServers)
      finishInitialRender()
    })
    .catch((reason: unknown) => {
      if (loadID !== loadSequence) {
        return
      }

      console.error(reason)
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
        caption:
          'Failed to load remote game servers: ' + ConnectErrorToString(ConnectError.from(reason)),
        icon: 'report_problem',
      })
    })
  const localServersRequest = xylonaClient
    .listGameServers(create(ListGameServersRequestSchema, {}))
    .then((response) => {
      if (loadID !== loadSequence) {
        return
      }

      bootstrapServers.value = buildLocalAggregatedServers(response.gameServers)
      finishInitialRender()
    })
    .catch((reason: unknown) => {
      if (loadID !== loadSequence) {
        return
      }

      console.error(reason)
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
        caption:
          'Failed to load local game servers: ' + ConnectErrorToString(ConnectError.from(reason)),
        icon: 'report_problem',
      })
      finishInitialRender()
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

  void aggregatedRequest
  void nodesRequest

  await Promise.race([localServersRequest, aggregatedRequest])
}

function watchServerStatusChanges() {
  XylonaEventBus.on('gameServerStatus', handleServerStatusUpdate)
}

function watchServerVersionChanges() {
  XylonaEventBus.on('gameServerVersion', handleServerVersionUpdate)
}

async function deleteGameServerAction(row: DisplayRow | null) {
  if (row !== null) {
    selectedGameServers.value = [row]
  }
  showDeleteGameServerDialog.value = true
}

async function deleteGameServerSubmitted(error: unknown | boolean) {
  showDeleteGameServerDialog.value = false
  selectedGameServers.value = []
  if (!error) {
    void getGameServers()
  }
}

function setServerStatus(serverID: string, serverStatus: Status) {
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
  const liveServerSets = [bootstrapServers.value, aggregatedServers.value]

  for (const liveServers of liveServerSets) {
    if (liveServers === null) {
      continue
    }

    for (const server of liveServers) {
      const didUpdate = updater(server)
      if (!didUpdate) {
        continue
      }

      const compositeID = getAggregatedServerCompositeID(server)
      if (compositeID !== null) {
        dirtyLiveServerCompositeIDs.add(compositeID)
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
  if (selectedGameServersForStart.value.length < 1 || loading.value) {
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
  if (selectedGameServersForStop.value.length < 1 || loading.value) {
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
</style>
