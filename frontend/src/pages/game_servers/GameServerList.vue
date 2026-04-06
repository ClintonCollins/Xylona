<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Game Servers</h1>
      <div class="xy-page-actions">
        <q-btn
          v-if="selectedGameServers.length >= 1"
          color="positive"
          :disable="loading || selectedGameServersForStart.length < 1"
          label="Start selected"
          @click="startSelectedGameServers" />
        <q-btn
          v-if="selectedGameServers.length >= 1"
          color="warning"
          :disable="loading || selectedGameServersForStop.length < 1"
          label="Stop selected"
          @click="stopSelectedGameServers" />
        <q-btn
          v-if="selectedGameServers.length >= 1"
          color="negative"
          :disable="loading"
          label="Remove game server"
          @click="deleteGameServerAction(null)" />
        <q-input
          v-model="search"
          dense
          outlined
          debounce="300"
          color="primary"
          placeholder="Search..."
          aria-label="Search game servers"
          class="xy-search-input">
          <template #append>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn
          v-if="showCreateButton"
          color="primary"
          to="/game-servers/create"
          :disable="loading"
          label="Create Game Server" />
      </div>
    </div>
    <div>
      <q-table
        v-model:pagination="initialPagination"
        v-model:selected="selectedGameServers"
        flat
        class="xy-standalone-table"
        :grid="$q.screen.lt.md"
        :rows="displayRows"
        :columns="columns"
        row-key="compositeId"
        selection="multiple"
        :filter="search"
        :loading="loading"
        hide-header-in-grid>
        <template #body-cell-name="props">
          <q-td :props="props">
            <router-link class="table-link" :to="'/game-servers/' + props.row.id + '/console'">
              {{ props.row.displayName }}
            </router-link>
            <q-badge v-if="props.row.isStale" color="warning" class="q-ml-xs" label="stale" />
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <status-badge style="margin-left: -1em" :status="props.row.statusEnum"></status-badge>
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
              <q-spinner size="1em" color="primary" />
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
                  flat
                  class="text-main-brighter"
                  :icon="tabSettings"
                  aria-label="Edit game server">
                  <q-tooltip>Edit game server</q-tooltip>
                </q-btn>
              </router-link>
              <span>
                <q-btn
                  flat
                  class="text-error-brighter"
                  :icon="tabTrash"
                  aria-label="Delete game server"
                  @click="deleteGameServerAction(props.row)">
                  <q-tooltip>Delete game server</q-tooltip>
                </q-btn>
              </span>
            </div>
          </q-td>
        </template>
        <template #no-data>
          <div class="full-width column items-center q-pa-lg text-xy-secondary">
            <q-icon name="dns" size="3rem" class="q-mb-sm text-xy-muted" />
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

<script setup lang="ts">
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
  extractRemoteNodeIDs,
  filterRowsByRemoteNodeIDs,
  type DisplayRow,
} from './server-list-cache'
import { getStartableServers, getStoppableServers } from './server-list-actions'
import { useUserAuthStore } from '@/stores/xylona'
import { resolveCanonicalVersionDisplay } from './version-display'

const aggregatedServers = ref([] as AggregatedGameServer[])
const nodesByID = ref(new Map<string, Node>())
const hasFetchedLiveRows = ref(false)
const loading: Ref<boolean> = ref(false)
const search: Ref<string> = ref('')
const showDeleteGameServerDialog = ref(false)
const selectedGameServers = ref([] as DisplayRow[])
const cachedDisplayRows = useStorage<DisplayRow[]>('game-server-display-rows-cache', [])
const cachedRemoteNodeIDs = useStorage<string[]>('game-server-remote-node-ids-cache', [])
const allowedRemoteNodeIDs = ref(new Set(cachedRemoteNodeIDs.value))
const $q = useQuasar()

const initialPagination = useStorage('game-server-pagination', {
  rowsPerPage: 25,
  page: 1,
})
const authStore = useUserAuthStore()
const showCreateButton = computed(() => authStore.user?.superUser ?? false)

const liveDisplayRows = computed((): DisplayRow[] => {
  return buildDisplayRows(aggregatedServers.value, nodesByID.value)
})

const displayRows = computed((): DisplayRow[] => {
  if (hasFetchedLiveRows.value) {
    return liveDisplayRows.value
  }
  return filterRowsByRemoteNodeIDs(cachedDisplayRows.value, allowedRemoteNodeIDs.value)
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
  loading.value = true
  try {
    const [serversResult, nodesResult] = await Promise.allSettled([
      GetXylonaClient().listAggregatedGameServers(
        create(ListAggregatedGameServersRequestSchema, {}),
      ),
      GetXylonaClient().listNodes(create(ListNodesRequestSchema, {})),
    ])

    if (serversResult.status === 'fulfilled') {
      aggregatedServers.value = serversResult.value.servers
      hasFetchedLiveRows.value = true
    } else {
      console.error(serversResult.reason)
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption:
          'Failed to load game servers: ' +
          ConnectErrorToString(ConnectError.from(serversResult.reason)),
        icon: 'report_problem',
      })
    }

    let remoteNodeIDs = new Set(cachedRemoteNodeIDs.value)
    if (nodesResult.status === 'fulfilled') {
      nodesByID.value = new Map(nodesResult.value.nodes.map((node) => [node.id, node]))
      remoteNodeIDs = extractRemoteNodeIDs(nodesResult.value.nodes)
      cachedRemoteNodeIDs.value = [...remoteNodeIDs]
      allowedRemoteNodeIDs.value = remoteNodeIDs
      cachedDisplayRows.value = filterRowsByRemoteNodeIDs(cachedDisplayRows.value, remoteNodeIDs)
    } else {
      console.error(nodesResult.reason)
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption:
          'Failed to load nodes: ' + ConnectErrorToString(ConnectError.from(nodesResult.reason)),
        icon: 'report_problem',
      })
      nodesByID.value = new Map()
    }

    if (serversResult.status === 'fulfilled') {
      // Strip versionInfo before caching: VersionInfo contains bigint fields that
      // JSON.stringify cannot serialize. Version info is re-fetched fresh from the
      // server on each page load, so caching it provides no benefit.
      cachedDisplayRows.value = filterRowsByRemoteNodeIDs(
        buildDisplayRows(serversResult.value.servers, nodesByID.value).map((row) => ({
          ...row,
          versionInfo: undefined,
        })),
        remoteNodeIDs,
      )
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
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

  for (const server of aggregatedServers.value) {
    if (server.isLocal && server.localServer && server.localServer.id === serverID) {
      server.localServer.status = serverStatus
    }
    if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
      server.remoteServer.status = serverStatus
    }
  }
}

function setServerVersion(serverID: string, version: string, versionInfo?: VersionInfo) {
  for (const row of cachedDisplayRows.value) {
    if (row.id === serverID) {
      row.version = version
    }
  }

  for (const server of aggregatedServers.value) {
    if (server.isLocal && server.localServer && server.localServer.id === serverID) {
      server.localServer.version = version
      server.localServer.versionInfo = versionInfo
    }
    if (!server.isLocal && server.remoteServer && server.remoteServer.remoteServerId === serverID) {
      server.remoteServer.version = version
      server.remoteServer.versionInfo = versionInfo
    }
  }
}

function handleServerStatusUpdate(serverID: string, serverStatus: Status) {
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
  let successCount = 0
  const failedServerNames: string[] = []

  try {
    for (const selectedServer of selectedGameServersForStart.value) {
      const request: StartGameServerRequest = create(StartGameServerRequestSchema, {})
      request.serverId = selectedServer.id
      try {
        await GetXylonaClient().startGameServer(request)
        setServerStatus(selectedServer.id, Status.ONLINE)
        successCount++
      } catch (errStart) {
        failedServerNames.push(selectedServer.displayName)
        console.error(errStart)
      }
    }
  } finally {
    loading.value = false
  }

  selectedGameServers.value = []

  if (successCount > 0) {
    $q.notify({
      caption: `Started ${successCount} game server${successCount === 1 ? '' : 's'}.`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
  }

  if (failedServerNames.length > 0) {
    $q.notify({
      caption: `Failed to start: ${failedServerNames.join(', ')}`,
      type: 'xylona-error',
      position: 'top',
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
  let successCount = 0
  const failedServerNames: string[] = []

  try {
    for (const selectedServer of selectedGameServersForStop.value) {
      const request: StopGameServerRequest = create(StopGameServerRequestSchema, {})
      request.serverId = selectedServer.id
      try {
        await GetXylonaClient().stopGameServer(request)
        setServerStatus(selectedServer.id, Status.OFFLINE)
        successCount++
      } catch (errStop) {
        failedServerNames.push(selectedServer.displayName)
        console.error(errStop)
      }
    }
  } finally {
    loading.value = false
  }

  selectedGameServers.value = []

  if (successCount > 0) {
    $q.notify({
      caption: `Stopped ${successCount} game server${successCount === 1 ? '' : 's'}.`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
  }

  if (failedServerNames.length > 0) {
    $q.notify({
      caption: `Failed to stop: ${failedServerNames.join(', ')}`,
      type: 'xylona-error',
      position: 'top',
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
