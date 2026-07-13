<template>
  <div class="start-args-page">
    <div class="start-args-page__header">
      <div class="start-args-page__header-main">
        <div class="start-args-page__eyebrow">Runtime</div>
        <h1 class="start-args-page__title font-display">Start Command</h1>
        <p class="start-args-page__copy text-xy-secondary">
          Edit the structured launch arguments for this server and preview the resolved argv before
          saving.
        </p>
      </div>
      <div class="start-args-page__actions">
        <q-btn
          :disable="saving || restarting || !isDirty || !canEditStartArgs"
          flat
          icon="restart_alt"
          label="Reset All"
          @click="resetAll" />
        <q-btn
          :disable="
            saving || restarting || !isDirty || !canEditStartArgs || !restartStateAuthoritative
          "
          :loading="restarting"
          color="warning"
          flat
          icon="replay"
          label="Save & Restart"
          @click="saveAndRestart">
          <q-tooltip v-if="restartUnavailableReason">
            {{ restartUnavailableReason }}
          </q-tooltip>
        </q-btn>
        <q-btn
          :disable="saving || restarting || !isDirty || !canEditStartArgs"
          :loading="saving"
          color="primary"
          icon="save"
          label="Save"
          @click="saveOnly" />
      </div>
    </div>

    <div v-if="loading" class="start-args-page__loading">
      <q-spinner-dots color="primary" size="40px" />
      <div class="text-xy-secondary q-mt-sm">Loading start command details...</div>
    </div>

    <div v-else class="start-args-page__body">
      <q-banner
        v-if="!effectiveAllowEditing"
        class="start-args-page__warning"
        inline-actions
        rounded>
        Start command editing is disabled for this game definition.
      </q-banner>

      <q-banner v-if="platformWarning" class="start-args-page__warning" inline-actions rounded>
        {{ platformWarning }}
      </q-banner>

      <div class="start-args-page__preview-rail">
        <resolved-command-preview
          :base-command="baseCommand"
          :resolved-blocks="resolvedPreview.resolvedBlocks" />
      </div>

      <div class="start-args-page__status-strip" aria-label="Start command state">
        <div class="start-args-page__status-item">
          <q-icon :name="platformIcon" class="start-args-page__status-icon" size="20px" />
          <div>
            <span class="start-args-page__status-label">Platform</span>
            <strong>{{ platformLabel }}</strong>
          </div>
        </div>
        <div class="start-args-page__status-item">
          <q-icon class="start-args-page__status-icon" name="format_list_numbered" size="20px" />
          <div>
            <span class="start-args-page__status-label">Template blocks</span>
            <strong>{{ templateBlockCount }}</strong>
          </div>
        </div>
        <div class="start-args-page__status-item">
          <q-icon class="start-args-page__status-icon" name="edit" size="20px" />
          <div>
            <span class="start-args-page__status-label">Draft state</span>
            <strong>{{ draftChangeLabel }}</strong>
          </div>
        </div>
        <div class="start-args-page__status-item">
          <q-icon class="start-args-page__status-icon" name="code" size="20px" />
          <div>
            <span class="start-args-page__status-label">Resolved argv</span>
            <strong>{{ resolvedTokenCount }} tokens</strong>
          </div>
        </div>
      </div>

      <div class="start-args-page__workspace">
        <div class="start-args-page__editor-column">
          <start-args-editor
            :allow-editing="canEditStartArgs"
            :base-command="baseCommand"
            :blocklist="blocklistEntries"
            :patches="draftPatches"
            :template="templateBlocks"
            @update:patches="draftPatches = $event" />
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import StartArgsEditor from '@/components/game_servers/StartArgsEditor.vue'
import ResolvedCommandPreview from '@/components/game_servers/ResolvedCommandPreview.vue'
import {
  buildPlaceholderVars,
  parseStartArgBlocklist,
  parseStartArgsPatches,
  parseStartArgsTemplate,
  resolveStartArgs,
  serializeStartArgsPatches,
  type StartArgPatch,
} from '@/components/game_servers/start-args'
import {
  StartGameServerRequestSchema,
  GameServerSchema,
  Status,
  StopGameServerRequestSchema,
} from '@/proto/shared_pb'
import {
  GetGameServerRequestSchema,
  type GetGameServerResponse,
  ListNodesRequestSchema,
  type ListNodesResponse,
  UpdateGameServerStartArgsRequestSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { resolveStartArgsPlatform } from './start-args-platform'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'

const $q = useQuasar()
const route = useRoute()
const router = useRouter()
const authStore = useUserAuthStore()

const gameServerId = ref(String(route.params.id ?? ''))
const loading = ref(true)
const saving = ref(false)
const restarting = ref(false)
const serverStatusFresh = ref(false)
const isSuperUser = ref(false)
const gameServer = ref<GetGameServerResponse['gameServer']>()
const nodeOsById = ref<Record<string, string>>({})
const draftPatches = ref<StartArgPatch[]>([])
const savedPatchesJson = ref('')
let liveStatusSequence = 0
let statusRefreshRequestSequence = 0

const selectedPlatform = computed<'linux' | 'windows' | null>(() =>
  resolveStartArgsPlatform(
    nodeOsById.value[gameServer.value?.nodeId ?? ''],
    Boolean(gameServer.value?.game?.linuxStartArgsTemplate),
    Boolean(gameServer.value?.game?.windowsStartArgsTemplate),
  ),
)

const templateBlocks = computed(() => {
  const game = gameServer.value?.game
  if (!game || selectedPlatform.value === null) {
    return []
  }

  return parseStartArgsTemplate(
    selectedPlatform.value === 'windows'
      ? game.windowsStartArgsTemplate
      : game.linuxStartArgsTemplate,
  )
})

const baseCommand = computed(() => {
  const game = gameServer.value?.game
  if (!game || selectedPlatform.value === null) {
    return ''
  }

  return selectedPlatform.value === 'windows' ? game.windowsBaseCommand : game.linuxBaseCommand
})

const blocklistEntries = computed(() =>
  parseStartArgBlocklist(gameServer.value?.game?.startArgBlocklist ?? ''),
)

const effectiveAllowEditing = computed(
  () => Boolean(gameServer.value?.game?.allowStartArgEditing) || isSuperUser.value,
)

const canEditStartArgs = computed(
  () => effectiveAllowEditing.value && selectedPlatform.value !== null,
)

const hasRestartPermissions = computed(() => {
  const permissions = gameServer.value?.effectivePermissions ?? []
  return (
    permissions.length === 0 ||
    (permissions.includes('game_server.start') && permissions.includes('game_server.stop'))
  )
})

const restartStateAuthoritative = computed(
  () =>
    websocketStateAuthoritative.value &&
    serverStatusFresh.value &&
    gameServer.value !== undefined &&
    gameServer.value.status === Status.ONLINE &&
    hasRestartPermissions.value,
)

const restartUnavailableReason = computed(() => {
  if (
    !websocketStateAuthoritative.value ||
    !serverStatusFresh.value ||
    gameServer.value?.status === Status.UNKNOWN
  ) {
    return 'Waiting for authoritative server status'
  }
  if (!hasRestartPermissions.value) {
    return 'Requires start and stop permissions'
  }
  if (gameServer.value?.status !== Status.ONLINE) {
    return 'The server must be online to restart'
  }
  return ''
})

const resolvedPreview = computed(() =>
  selectedPlatform.value === null
    ? { args: [], resolvedBlocks: [] }
    : resolveStartArgs(
        templateBlocks.value,
        draftPatches.value,
        buildPlaceholderVars(gameServer.value ?? undefined),
      ),
)

const platformLabel = computed(() => {
  if (selectedPlatform.value === null) {
    return 'Unknown'
  }

  return selectedPlatform.value === 'windows' ? 'Windows' : 'Linux'
})

const platformIcon = computed(() => {
  if (selectedPlatform.value === 'windows') {
    return 'desktop_windows'
  }

  return selectedPlatform.value === 'linux' ? 'terminal' : 'help_outline'
})

const templateBlockCount = computed(() => templateBlocks.value.length)
const draftChangeCount = computed(() => draftPatches.value.length)
const draftChangeLabel = computed(() => {
  if (draftChangeCount.value === 0) {
    return 'Clean'
  }

  return draftChangeCount.value === 1 ? '1 change' : `${draftChangeCount.value} changes`
})
const resolvedTokenCount = computed(() =>
  resolvedPreview.value.resolvedBlocks.reduce(
    (count, block) => count + block.resolvedTokens.length,
    baseCommand.value === '' ? 0 : 1,
  ),
)

const isDirty = computed(
  () => serializeStartArgsPatches(draftPatches.value) !== savedPatchesJson.value,
)

const platformWarning = computed(() => {
  if (selectedPlatform.value === null) {
    return 'Unable to determine this server platform. Reload node data before editing start arguments.'
  }

  if (baseCommand.value !== '' || templateBlocks.value.length > 0) {
    return ''
  }

  return `No ${selectedPlatform.value} start template is configured for this game.`
})

onMounted(async () => {
  XylonaEventBus.on('gameServerStatus', onServerStatus)
  XylonaEventBus.on('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.on('websocketDisconnected', onWebsocketDisconnect)
  await initialize()
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServerStatus', onServerStatus)
  XylonaEventBus.off('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', onWebsocketDisconnect)
})

async function initialize() {
  loading.value = true

  try {
    const authResponse = await authStore.checkUserAuthenticated()
    isSuperUser.value = authResponse?.user?.superUser ?? authStore.user?.superUser ?? false

    await Promise.all([loadGameServer(), loadNodes()])

    if (!effectiveAllowEditing.value) {
      await router.replace(`/game-servers/${gameServerId.value}/console`)
      return
    }
  } finally {
    loading.value = false
  }
}

async function loadGameServer() {
  const response = await GetXylonaClient().getGameServer(
    create(GetGameServerRequestSchema, {
      id: gameServerId.value,
    }),
  )
  gameServer.value = response.gameServer
  serverStatusFresh.value = websocketStateAuthoritative.value
  savedPatchesJson.value = gameServer.value?.startArgsPatches ?? ''
  draftPatches.value = parseStartArgsPatches(savedPatchesJson.value)
}

function onServerStatus(serverID: string, _serverName: string, status: Status) {
  if (serverID !== gameServerId.value || !gameServer.value) {
    return
  }

  liveStatusSequence++
  gameServer.value = create(GameServerSchema, {
    ...gameServer.value,
    status,
  })
  serverStatusFresh.value = websocketStateAuthoritative.value
}

function onWebsocketDisconnect() {
  serverStatusFresh.value = false
}

async function onWebsocketReconnect() {
  const requestSequence = ++statusRefreshRequestSequence
  const statusSequenceAtStart = liveStatusSequence
  serverStatusFresh.value = false
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerId.value }),
    )
    if (
      requestSequence !== statusRefreshRequestSequence ||
      !response.gameServer ||
      !gameServer.value
    ) {
      return
    }
    gameServer.value = create(GameServerSchema, {
      ...gameServer.value,
      effectivePermissions: response.gameServer.effectivePermissions,
      status:
        statusSequenceAtStart === liveStatusSequence
          ? response.gameServer.status
          : gameServer.value.status,
    })
    serverStatusFresh.value = websocketStateAuthoritative.value
  } catch (unknownError: unknown) {
    if (requestSequence !== statusRefreshRequestSequence) {
      return
    }
    if (gameServer.value) {
      gameServer.value = create(GameServerSchema, {
        ...gameServer.value,
        status: Status.UNKNOWN,
      })
    }
    const err = ConnectError.from(unknownError)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: `Failed to refresh server status: ${ConnectErrorToString(err)}`,
      icon: 'report_problem',
    })
  }
}

async function loadNodes() {
  try {
    const response: ListNodesResponse = await GetXylonaClient().listNodes(
      create(ListNodesRequestSchema),
    )
    nodeOsById.value = Object.fromEntries(response.nodes.map((node) => [node.id, node.os]))
  } catch {
    nodeOsById.value = {}
  }
}

async function saveOnly() {
  await savePatches(false)
}

async function saveAndRestart() {
  if (!restartStateAuthoritative.value) {
    return
  }
  await savePatches(true)
}

async function savePatches(restartAfterSave: boolean) {
  if (!gameServer.value) {
    return
  }

  if (!canEditStartArgs.value) {
    return
  }

  if (restartAfterSave) {
    restarting.value = true
  } else {
    saving.value = true
  }

  let startArgsSaved = false
  try {
    const response = await GetXylonaClient().updateGameServerStartArgs(
      create(UpdateGameServerStartArgsRequestSchema, {
        serverId: gameServerId.value,
        startArgsPatches: serializeStartArgsPatches(draftPatches.value),
      }),
    )
    const runtimeStatus = gameServer.value.status
    gameServer.value = response.gameServer
      ? create(GameServerSchema, {
          ...response.gameServer,
          status: runtimeStatus,
        })
      : response.gameServer
    savedPatchesJson.value = response.gameServer?.startArgsPatches ?? ''
    draftPatches.value = parseStartArgsPatches(savedPatchesJson.value)
    startArgsSaved = true

    if (restartAfterSave) {
      const client = GetXylonaClient()
      await client.stopGameServer(
        create(StopGameServerRequestSchema, { serverId: gameServerId.value }),
      )
      await waitForServerOffline()
      await client.startGameServer(
        create(StartGameServerRequestSchema, { serverId: gameServerId.value }),
      )
      $q.notify({
        type: 'positive',
        position: 'top',
        caption: 'Start arguments saved and server restarted.',
        icon: 'task_alt',
      })
      return
    }

    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Start arguments saved successfully.',
      icon: 'task_alt',
    })
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: startArgsSaved
        ? `Start arguments were saved, but the server could not restart: ${ConnectErrorToString(err)}`
        : `Failed to save start arguments: ${ConnectErrorToString(err)}`,
      icon: 'report_problem',
    })
  } finally {
    saving.value = false
    restarting.value = false
  }
}

async function waitForServerOffline() {
  const timeoutAt = Date.now() + 30_000
  while (Date.now() < timeoutAt) {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerId.value }),
    )
    if (!response.gameServer) {
      throw new Error('Game server details were unavailable while waiting for shutdown')
    }

    gameServer.value = response.gameServer
    if (response.gameServer.status === Status.OFFLINE) {
      return
    }
    if (response.gameServer.status === Status.UNKNOWN) {
      throw new Error('Game server status became unavailable while waiting for shutdown')
    }

    await new Promise((resolve) => setTimeout(resolve, 500))
  }

  throw new Error('Timed out waiting for the game server to stop')
}

function resetAll() {
  draftPatches.value = []
}
</script>

<style scoped>
.start-args-page {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
  padding: var(--xy-space-xl);
}

.start-args-page__header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
}

.start-args-page__header-main {
  min-width: min(100%, 34rem);
}

.start-args-page__eyebrow {
  font-size: 0.74rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.start-args-page__title {
  margin: 0;
  font-size: 1.55rem;
  line-height: 1.15;
  color: var(--xy-text-primary);
}

.start-args-page__copy {
  margin: 6px 0 0;
  max-width: 66ch;
  font-size: 0.88rem;
  line-height: 1.5;
}

.start-args-page__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.start-args-page__status-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background: var(--xy-border);
}

.start-args-page__status-item {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: var(--xy-space-sm);
  align-items: center;
  min-height: 72px;
  padding: var(--xy-space-md);
  background:
    var(--xy-surface-gradient-subtle),
    color-mix(in srgb, var(--xy-surface-0) 78%, var(--xy-surface-1) 22%);
}

.start-args-page__status-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--xy-accent);
}

.start-args-page__status-label {
  display: block;
  color: var(--xy-text-muted);
  font-size: 0.72rem;
  line-height: 1.3;
}

.start-args-page__status-item strong {
  display: block;
  margin-top: 2px;
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.86rem;
  font-weight: 600;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.start-args-page__loading {
  display: flex;
  min-height: 280px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.start-args-page__body {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.start-args-page__preview-rail {
  position: sticky;
  top: var(--xy-space-sm);
  z-index: 5;
  min-width: 0;
}

.start-args-page__workspace {
  min-width: 0;
}

.start-args-page__editor-column {
  min-width: 0;
}

.start-args-page__warning {
  background: var(--xy-warning-bg-soft);
  border: 1px solid var(--xy-warning-border);
  color: var(--xy-text-primary);
}

@media (max-width: 1160px) {
  .start-args-page__status-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .start-args-page {
    padding: var(--xy-space-md);
  }

  .start-args-page__actions,
  .start-args-page__actions :deep(.q-btn) {
    width: 100%;
  }

  .start-args-page__status-strip {
    grid-template-columns: 1fr;
  }

  .start-args-page__preview-rail {
    top: var(--xy-space-xs);
  }
}
</style>
