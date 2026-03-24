<template>
  <div class="server-form-shell full-width" data-testid="game-server-form-shell">
    <div ref="stickySentinel" class="sticky-sentinel"></div>

    <div
      class="server-form-header"
      :class="{ 'is-stuck': isStuck, 'is-compact': isCompactEditHeader }">
      <div class="server-form-header-left">
        <div v-if="!isCompactEditHeader" class="server-form-breadcrumbs">
          <router-link to="/game-servers" class="breadcrumb-link">Game Servers</router-link>
          <span class="breadcrumb-sep">/</span>
          <span class="breadcrumb-current">{{ breadcrumbLabel }}</span>
        </div>
        <div class="server-form-title font-display">{{ headerTitle }}</div>
        <div v-if="!isCompactEditHeader" class="server-form-subtitle text-xy-secondary">
          {{ formSubtitle }}
        </div>
      </div>

      <div class="server-form-header-actions">
        <q-btn flat label="Cancel" :disable="formSubmitting" @click="cancel" />
        <q-btn
          class="server-form-save-btn"
          :class="{ 'server-form-save-btn--ready': deploymentReady }"
          label="Save"
          color="primary"
          :loading="formSubmitting"
          :disable="loading"
          @click="submitGameServer" />
      </div>
    </div>

    <div v-if="loading" class="server-form-loading">
      <q-spinner-dots size="40px" color="primary" />
      <div class="text-xy-secondary q-mt-sm">Loading server options...</div>
    </div>

    <div v-else class="server-form-body">
      <div class="server-form-guidance text-xy-muted">
        Fields marked <span class="server-form-guidance-mark">*</span> are required.
      </div>

      <q-form ref="formRef" greedy class="server-form-layout">
        <section class="form-section form-section--animated" style="--section-index: 0">
          <div class="section-header">
            <span class="section-icon section-icon--accent" data-testid="section-icon-identity">
              <q-icon name="badge" size="14px" />
            </span>
            <span class="section-title font-display">Identity</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model="gameServer.name"
              class="col-12 col-md-6"
              outlined
              type="text"
              autofocus
              label="Server Name *"
              :rules="serverNameRules"
              reactive-rules
              lazy-rules
              maxlength="80"
              hint="Use the name players will recognize." />
            <q-select
              v-model="gameServer.gameId"
              class="col-12 col-md-6"
              outlined
              label="Game *"
              emit-value
              :options="availableGames"
              option-label="label"
              map-options
              options-selected-class="selected-option"
              :rules="gameRules"
              reactive-rules
              lazy-rules
              hint="Changing the game updates the default ports and player limits."
              @update:model-value="onGameSelected" />
          </div>
        </section>

        <section class="form-section form-section--animated" style="--section-index: 1">
          <div class="section-header">
            <span class="section-icon section-icon--primary" data-testid="section-icon-placement">
              <q-icon name="hub" size="14px" />
            </span>
            <span class="section-title font-display">Placement</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-select
              v-model="gameServer.userId"
              class="col-12 col-md-4"
              outlined
              label="Owner *"
              emit-value
              :options="availableUsers"
              option-label="label"
              map-options
              options-selected-class="selected-option"
              :rules="ownerRules"
              reactive-rules
              lazy-rules />
            <q-select
              v-model="gameServer.nodeId"
              class="col-12 col-md-4"
              outlined
              label="Node *"
              emit-value
              :options="nodes"
              option-label="name"
              map-options
              option-value="id"
              options-selected-class="selected-option"
              :rules="nodeRules"
              reactive-rules
              lazy-rules />
            <q-select
              v-model="gameServer.ip"
              class="col-12 col-md-4"
              outlined
              label="IP Address *"
              :options="availableIPs"
              option-label="address"
              options-selected-class="selected-option"
              :rules="ipRules"
              reactive-rules
              lazy-rules
              hint="Choose the address players will use." />
          </div>
        </section>

        <section class="form-section form-section--animated" style="--section-index: 2">
          <div class="section-header">
            <span class="section-icon section-icon--success" data-testid="section-icon-networking">
              <q-icon name="lan" size="14px" />
            </span>
            <span class="section-title font-display">Networking</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model.number="portModel"
              class="col-12 col-sm-6"
              outlined
              type="number"
              label="Port *"
              :rules="portRules"
              reactive-rules
              lazy-rules
              hint="Players connect here. Use 1 to 65535." />
            <q-input
              v-model.number="queryPortModel"
              class="col-12 col-sm-6"
              outlined
              type="number"
              label="Query Port *"
              :rules="queryPortRules"
              reactive-rules
              lazy-rules
              hint="Used for status queries. Use 1 to 65535." />
          </div>
        </section>

        <section
          class="form-section form-section--animated"
          :class="{ 'form-section--last': !isEditing }"
          style="--section-index: 3">
          <div class="section-header">
            <span class="section-icon section-icon--warning" data-testid="section-icon-capacity">
              <q-icon name="memory" size="14px" />
            </span>
            <span class="section-title font-display">Capacity</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model.number="setPlayersModel"
              class="col-12 col-sm-6 col-lg-4"
              outlined
              type="number"
              label="Set Players *"
              :rules="setPlayersRules"
              reactive-rules
              lazy-rules
              hint="Initial player limit reported by the server." />
            <q-input
              v-model.number="maxPlayersModel"
              class="col-12 col-sm-6 col-lg-4"
              outlined
              type="number"
              label="Max Players *"
              :rules="maxPlayersRules"
              reactive-rules
              lazy-rules
              hint="Maximum concurrent players." />
            <q-input
              v-if="isMinecraftGame"
              v-model.number="maxMemoryModel"
              class="col-12 col-lg-4"
              outlined
              type="number"
              label="Max Memory MB *"
              :rules="maxMemoryRules"
              :error="showMaxMemoryStateError"
              :error-message="maxMemoryStateMessage"
              reactive-rules
              lazy-rules
              hint="Set the RAM limit for this server." />
          </div>
        </section>

        <section
          v-if="isEditing"
          class="form-section form-section--last form-section--animated"
          style="--section-index: 4">
          <div class="section-header">
            <span class="section-icon section-icon--accent">
              <q-icon name="terminal" size="14px" />
            </span>
            <span class="section-title font-display">Runtime</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model="gameServer.startCommand"
              class="col-12"
              outlined
              type="text"
              label="Start Command"
              hint="Only override this if the default launch command is wrong." />
          </div>
        </section>

        <section
          v-if="!isEditing"
          class="form-section form-section--summary form-section--animated"
          style="--section-index: 4">
          <Transition name="deployment-state" mode="out-in">
            <div v-if="deploymentReady" key="ready" class="deployment-ready">
              <div class="deployment-ready-icon">
                <q-icon name="task_alt" size="16px" />
              </div>
              <div class="deployment-ready-content">
                <span class="deployment-ready-label font-display"> Ready to Deploy </span>
                <span class="deployment-ready-value">{{ deploymentReadyText }}</span>
              </div>
            </div>

            <div v-else key="review" class="deployment-review">
              <div class="deployment-review-heading">
                <span class="section-icon section-icon--muted">
                  <q-icon name="fact_check" size="14px" />
                </span>
                <div class="deployment-review-copy">
                  <span class="deployment-review-title font-display">
                    Needs Attention
                    <span class="deployment-review-dot" aria-hidden="true"></span>
                  </span>
                  <span class="deployment-review-subtitle text-xy-muted">
                    Only blocking or conflicting setup appears here.
                  </span>
                </div>
              </div>

              <div class="deployment-summary">
                <article
                  v-for="item in deploymentWarningItems"
                  :key="item.label"
                  class="deployment-summary-item deployment-summary-item--warning">
                  <div class="deployment-summary-icon is-warning">
                    <q-icon :name="item.icon" size="15px" />
                  </div>
                  <div class="deployment-summary-content">
                    <span class="deployment-summary-label">{{ item.label }}</span>
                    <span class="deployment-summary-value">{{ item.value }}</span>
                  </div>
                </article>
              </div>
            </div>
          </Transition>
        </section>
      </q-form>
    </div>

    <q-inner-loading
      :showing="formSubmitting"
      label="Saving game server..."
      label-class="text-primary" />
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { QForm } from 'quasar'
import { useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  describeMinecraftMemoryState,
  validateMaxMemory,
  validatePlayerCount,
  validatePlayerCountAtMost,
  validatePort,
  validateRequiredSelection,
  validateRequiredText,
  validateRequiredValue,
} from './game-server-form-validation'
import {
  CreateGameServerRequest,
  CreateGameServerRequestSchema,
  EditGameServerRequest,
  EditGameServerRequestSchema,
  Game,
  GameServer,
  GameServerSchema,
  IP,
  Node,
} from '@/proto/shared_pb'
import {
  GetGameServerRequest,
  GetGameServerRequestSchema,
  ListGamesRequest,
  ListGamesRequestSchema,
  ListGamesResponse,
  ListIPsRequest,
  ListIPsRequestSchema,
  ListIPsResponse,
  ListNodesRequest,
  ListNodesRequestSchema,
  ListNodesResponse,
  ListUsersRequest,
  ListUsersRequestSchema,
} from '@/proto/xylona_pb'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()

const props = defineProps({
  existingGameServerId: {
    type: String,
    required: false,
    default: undefined,
  },
})

const gameServer = ref(create(GameServerSchema, {}))
const availableGames = ref<Array<Record<string, string>>>([])
const availableUsers = ref<Array<Record<string, string>>>([])
const availableIPs = ref<Array<IP>>([])
const gamesMap = ref(new Map<string, Game>())
const nodes = ref<Array<Node>>([])

const formSubmitting = ref(false)
const loading = ref(true)
const stickySentinel = ref<HTMLElement | null>(null)
const isStuck = ref(false)
const formRef = ref<QForm | null>(null)

let stickyObserver: IntersectionObserver | undefined

const isEditing = computed(() => props.existingGameServerId !== undefined)

const breadcrumbLabel = computed(() => {
  if (!isEditing.value) {
    return 'New Server'
  }
  return gameServer.value.name || 'Edit Server'
})

const formTitle = computed(() => {
  if (!isEditing.value) {
    return 'Create Game Server'
  }
  return gameServer.value.name ? `Editing ${gameServer.value.name}` : 'Edit Game Server'
})
const isSettingsContext = computed(() => route.path.endsWith('/settings'))
const isCompactEditHeader = computed(() => isEditing.value && isSettingsContext.value)
const headerTitle = computed(() => {
  if (!isCompactEditHeader.value) {
    return formTitle.value
  }

  return gameServer.value.name || 'Server Settings'
})

const formSubtitle = computed(() =>
  isEditing.value
    ? 'Update ownership, routing, and limits from the server page.'
    : 'Set the server route and limits. The footer only flags what still blocks save.',
)
const isMinecraftGame = computed(() => gameServer.value.gameId === 'minecraft')
const maxMemoryStateMessage = computed(() =>
  isMinecraftGame.value ? (describeMinecraftMemoryState(maxMemoryModel.value) ?? '') : '',
)
const showMaxMemoryStateError = computed(
  () => isEditing.value && isMinecraftGame.value && maxMemoryStateMessage.value.length > 0,
)

const trimmedServerName = computed(() => gameServer.value.name?.trim() ?? '')

const selectedGameName = computed(
  () =>
    gamesMap.value.get(gameServer.value.gameId)?.name ??
    gameServer.value.gameName ??
    'Choose a game',
)

const selectedNodeName = computed(
  () =>
    nodes.value.find((node) => node.id === gameServer.value.nodeId)?.name ??
    gameServer.value.nodeName ??
    'Choose a node',
)

const selectedOwnerName = computed(() => {
  const owner = availableUsers.value.find((user) => user.value === gameServer.value.userId)
  return owner?.label ?? gameServer.value.userName ?? 'Choose an owner'
})

const selectedIPLabel = computed(() => gameServer.value.ip?.address || 'Choose an IP')

const portModel = computed({
  get: () => Number(gameServer.value.port ?? 0n),
  set: (value: number | string | null | undefined) => {
    gameServer.value.port = toBigInt(value)
  },
})

const queryPortModel = computed({
  get: () => Number(gameServer.value.queryPort ?? 0n),
  set: (value: number | string | null | undefined) => {
    gameServer.value.queryPort = toBigInt(value)
  },
})

const setPlayersModel = computed({
  get: () => Number(gameServer.value.setMaxPlayers ?? 0n),
  set: (value: number | string | null | undefined) => {
    gameServer.value.setMaxPlayers = toBigInt(value)
  },
})

const maxPlayersModel = computed({
  get: () => Number(gameServer.value.maxPlayers ?? 0n),
  set: (value: number | string | null | undefined) => {
    gameServer.value.maxPlayers = toBigInt(value)
  },
})

const maxMemoryModel = computed({
  get: () => Number(gameServer.value.maxMemoryMb ?? 0n),
  set: (value: number | string | null | undefined) => {
    gameServer.value.maxMemoryMb = toBigInt(value)
  },
})

const deploymentReviewItems = computed(() => [
  {
    label: 'Identity',
    value: trimmedServerName.value || 'Add a server name',
    icon: 'badge',
    warning: trimmedServerName.value.length === 0 || !gameServer.value.gameId,
  },
  {
    label: 'Placement',
    value: `${selectedOwnerName.value} on ${selectedNodeName.value}`,
    icon: 'dns',
    warning: !gameServer.value.userId || !gameServer.value.nodeId,
  },
  {
    label: 'Network',
    value:
      queryPortModel.value > 0 && queryPortModel.value !== portModel.value
        ? `${selectedIPLabel.value}:${portModel.value || 0} / query ${queryPortModel.value}`
        : `${selectedIPLabel.value}:${portModel.value || 0}`,
    icon: 'lan',
    warning: !gameServer.value.ip?.address || portModel.value <= 0 || queryPortModel.value <= 0,
  },
  {
    label: 'Capacity',
    value: isMinecraftGame.value
      ? `${maxPlayersModel.value || 0} slots / ${maxMemoryModel.value || 0} MB`
      : `${maxPlayersModel.value || 0} slots / start ${setPlayersModel.value || 0}`,
    icon: 'memory',
    warning:
      maxPlayersModel.value <= 0 ||
      setPlayersModel.value > maxPlayersModel.value ||
      (isMinecraftGame.value && maxMemoryModel.value < 128),
  },
])

const deploymentWarningItems = computed(() =>
  deploymentReviewItems.value.filter((item) => item.warning),
)

const deploymentReady = computed(() => deploymentWarningItems.value.length === 0)

const deploymentReadyText = computed(() => {
  const capacitySummary = isMinecraftGame.value
    ? `${maxPlayersModel.value || 0} slots, ${maxMemoryModel.value || 0} MB`
    : `${maxPlayersModel.value || 0} slots`

  return `${trimmedServerName.value} for ${selectedGameName.value} on ${selectedNodeName.value} at ${selectedIPLabel.value}:${portModel.value || 0} with ${capacitySummary}`
})

const serverNameRules = [
  (value: string | null | undefined) => validateRequiredText(value, 'Server Name'),
]

const gameRules = [(value: string | null | undefined) => validateRequiredSelection(value, 'Game')]

const ownerRules = [(value: string | null | undefined) => validateRequiredSelection(value, 'Owner')]

const nodeRules = [(value: string | null | undefined) => validateRequiredSelection(value, 'Node')]

const ipRules = [(value: IP | null | undefined) => validateRequiredValue(value, 'IP Address')]

const portRules = [(value: number | string | bigint | null | undefined) => validatePort(value)]

const queryPortRules = [(value: number | string | bigint | null | undefined) => validatePort(value)]

const setPlayersRules = [
  (value: number | string | bigint | null | undefined) =>
    validatePlayerCount(value, 'Set Players', { minimum: 0 }),
  (value: number | string | bigint | null | undefined) =>
    validatePlayerCountAtMost(value, 'Set Players', maxPlayersModel.value, 'Max Players'),
]

const maxPlayersRules = [
  (value: number | string | bigint | null | undefined) =>
    validatePlayerCount(value, 'Max Players', { minimum: 1 }),
]

const maxMemoryRules = [
  (value: number | string | bigint | null | undefined) => validateMaxMemory(value),
]

onMounted(async () => {
  if (stickySentinel.value) {
    stickyObserver = new IntersectionObserver(
      ([entry]) => {
        isStuck.value = !entry.isIntersecting
      },
      { threshold: 0 },
    )
    stickyObserver.observe(stickySentinel.value)
  }

  loading.value = true

  try {
    if (isEditing.value) {
      await getGameServerDetails()
    }

    await Promise.all([getGames(), getNodes(), getUsers(), getIPs()])
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  stickyObserver?.disconnect()
})

function toBigInt(value: number | string | null | undefined): bigint {
  const numericValue =
    typeof value === 'number' ? value : value === null || value === undefined ? 0 : Number(value)

  if (!Number.isFinite(numericValue) || Number.isNaN(numericValue)) {
    return 0n
  }

  return BigInt(Math.max(0, Math.trunc(numericValue)))
}

function applyGameDefaults(game: Game) {
  gameServer.value.gameId = game.id
  gameServer.value.gameName = game.name
  gameServer.value.port = game.defaultPort ?? 0n
  gameServer.value.queryPort = game.defaultQueryPort ?? 0n
  gameServer.value.maxPlayers = game.defaultMaxPlayers ?? 0n
  gameServer.value.setMaxPlayers = game.defaultMaxPlayers ?? 0n

  if (game.id === 'minecraft' && gameServer.value.maxMemoryMb === 0n) {
    gameServer.value.maxMemoryMb = 1024n
  } else if (game.id !== 'minecraft') {
    gameServer.value.maxMemoryMb = 0n
  }
}

function onGameSelected(gameId: string) {
  const selectedGame = gamesMap.value.get(gameId)
  if (!selectedGame) {
    return
  }

  applyGameDefaults(selectedGame)
}

async function cancel() {
  router.back()
}

async function getGameServerDetails() {
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})

  try {
    request.id = props.existingGameServerId
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }

    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load game server details: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function getGames() {
  const request: ListGamesRequest = create(ListGamesRequestSchema, {})

  try {
    availableGames.value = []
    gamesMap.value = new Map<string, Game>()

    const response: ListGamesResponse = await GetXylonaClient().listGames(request)
    response.games.forEach((game) => {
      availableGames.value.push({ label: game.name, value: game.id })
      gamesMap.value.set(game.id, game)
    })

    if (availableGames.value.length === 0) {
      return
    }

    const existingSelection = gameServer.value.gameId
      ? gamesMap.value.get(gameServer.value.gameId)
      : undefined

    if (existingSelection) {
      gameServer.value.gameName = existingSelection.name
      return
    }

    const firstGame = gamesMap.value.get(availableGames.value[0].value)
    if (!firstGame) {
      return
    }

    applyGameDefaults(firstGame)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load games: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function getNodes() {
  const request: ListNodesRequest = create(ListNodesRequestSchema, {})

  try {
    const response: ListNodesResponse = await GetXylonaClient().listNodes(request)
    nodes.value = []
    response.nodes.forEach((node) => {
      nodes.value.push(node)
    })

    if (gameServer.value.nodeId) {
      return
    }

    const localNode = nodes.value.find((node) => node.local)
    if (localNode) {
      gameServer.value.nodeId = localNode.id
      gameServer.value.nodeName = localNode.name
      return
    }

    if (nodes.value.length > 0) {
      gameServer.value.nodeId = nodes.value[0].id
      gameServer.value.nodeName = nodes.value[0].name
    }
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    console.error(err.message)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load nodes: ' + ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  }
}

async function getUsers() {
  const request: ListUsersRequest = create(ListUsersRequestSchema, {})

  try {
    const response = await GetXylonaClient().listUsers(request)
    availableUsers.value = []
    response.users.forEach((user) => {
      availableUsers.value.push({ label: user.userName, value: user.id })
    })

    if (availableUsers.value.length === 0 || gameServer.value.userId) {
      return
    }

    gameServer.value.userId = availableUsers.value[0].value
    gameServer.value.userName = availableUsers.value[0].label
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load users: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function getIPs() {
  const request: ListIPsRequest = create(ListIPsRequestSchema, {})

  try {
    const response: ListIPsResponse = await GetXylonaClient().listIPs(request)
    availableIPs.value = []
    response.ips.forEach((ip) => {
      availableIPs.value.push(ip)
    })

    if (gameServer.value.ip?.address) {
      return
    }

    const preferredExternalIP = response.ips.find((ip) => ip.external)
    if (preferredExternalIP) {
      gameServer.value.ip = preferredExternalIP
      return
    }

    if (response.ips.length > 0) {
      gameServer.value.ip = response.ips[0]
    }
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load IP addresses: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function updateGameServer() {
  const request: EditGameServerRequest = create(EditGameServerRequestSchema, {})
  request.gameServer = gameServer.value as GameServer
  request.serverId = props.existingGameServerId

  try {
    const response = await GetXylonaClient().editGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to save game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function createGameServer() {
  const request: CreateGameServerRequest = create(CreateGameServerRequestSchema, {})
  request.gameServer = gameServer.value as GameServer

  try {
    const response = await GetXylonaClient().createGameServer(request)
    await router.push(`/game-servers/${response.gameServer?.id}/console`)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to create game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function submitGameServer() {
  if (!isMinecraftGame.value) {
    gameServer.value.maxMemoryMb = 0n
  }

  const formValid = (await formRef.value?.validate()) ?? true
  if (!formValid) {
    $q.notify({
      type: 'warning',
      position: 'top',
      caption: 'Complete the required fields before saving this server.',
      icon: 'report_problem',
    })
    return
  }

  formSubmitting.value = true

  try {
    if (isEditing.value) {
      await updateGameServer()
      return
    }

    await createGameServer()
  } finally {
    formSubmitting.value = false
  }
}
</script>

<style scoped>
.server-form-shell {
  width: 100%;
  --server-form-ease-out: cubic-bezier(0.22, 1, 0.36, 1);
  --server-form-ease-smooth: cubic-bezier(0.25, 1, 0.5, 1);
}

.sticky-sentinel {
  height: 0;
  overflow: hidden;
}

.server-form-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  padding: var(--xy-space-lg) var(--xy-space-lg) var(--xy-space-md);
  background:
    linear-gradient(180deg, var(--xy-accent-glow-soft), transparent 55%), var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
  border-radius: 10px 10px 0 0;
  position: sticky;
  top: 50px;
  z-index: 10;
  transition:
    border-color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast),
    background var(--xy-transition-fast),
    transform 220ms var(--server-form-ease-smooth);
}

.server-form-header.is-stuck {
  border-bottom-color: var(--xy-accent-border-soft);
  box-shadow: var(--xy-shadow-sticky-lg);
  transform: translateY(-2px);
}

.server-form-header.is-compact {
  align-items: center;
  gap: var(--xy-space-md);
  padding-top: var(--xy-space-md);
  padding-bottom: var(--xy-space-sm);
}

.server-form-header.is-compact .server-form-header-left {
  gap: 2px;
}

.server-form-header.is-compact .server-form-title {
  font-size: clamp(1.08rem, 0.98rem + 0.46vw, 1.34rem);
  line-height: 1.1;
}

.server-form-header.is-compact .server-form-header-actions {
  padding-top: 0;
}

.server-form-header-left {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.server-form-breadcrumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
}

.breadcrumb-link {
  color: var(--xy-text-muted);
  text-decoration: none;
  transition: color var(--xy-transition-fast);
}

.breadcrumb-link:hover {
  color: var(--xy-accent);
}

.breadcrumb-sep {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

.breadcrumb-current {
  color: var(--xy-text-secondary);
}

.server-form-title {
  font-size: clamp(1.28rem, 1.06rem + 0.8vw, 1.68rem);
  font-weight: 600;
  color: var(--xy-text-primary);
  letter-spacing: 0.015em;
  line-height: 1.15;
}

.server-form-subtitle {
  max-width: 64ch;
  font-size: 0.9rem;
  line-height: 1.6;
  color: var(--xy-text-secondary);
}

.server-form-header-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  flex-shrink: 0;
  padding-top: 2px;
}

.server-form-save-btn {
  transition:
    transform 180ms var(--server-form-ease-smooth),
    box-shadow 180ms var(--server-form-ease-smooth),
    filter 180ms var(--server-form-ease-smooth);
}

.server-form-save-btn--ready {
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--xy-success) 22%, transparent);
}

.server-form-save-btn--ready:hover {
  transform: translateY(-1px);
  filter: saturate(1.06);
}

.server-form-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 260px;
  padding: var(--xy-space-xl);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 10px 10px;
}

.server-form-body {
  padding: var(--xy-space-md) var(--xy-space-lg) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 10px 10px;
}

.server-form-guidance {
  margin-bottom: var(--xy-space-sm);
  font-size: 0.82rem;
  line-height: 1.45;
}

.server-form-guidance-mark {
  color: var(--xy-warning);
  font-weight: 600;
}

.server-form-layout {
  display: flex;
  flex-direction: column;
}

.form-section {
  padding: var(--xy-space-lg) 0;
  border-bottom: 1px solid var(--xy-border);
}

@media (prefers-reduced-motion: no-preference) {
  .form-section--animated {
    opacity: 0;
    transform: translateY(14px);
    animation: section-rise 480ms var(--server-form-ease-out) forwards;
    animation-delay: calc(60ms + (var(--section-index, 0) * 85ms));
  }

  .server-form-save-btn--ready {
    animation: save-ready-breathe 2.4s var(--server-form-ease-out) infinite;
  }
}

.form-section:first-child {
  padding-top: var(--xy-space-sm);
}

.form-section--last {
  border-bottom: none;
  padding-bottom: 0;
}

.form-section--summary {
  padding-top: clamp(1.5rem, 1.1rem + 1vw, 2rem);
  border-top: 1px solid var(--xy-border);
  border-bottom: none;
  margin-top: var(--xy-space-md);
  padding-bottom: 0;
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-sm);
}

.section-title {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--xy-text-emphasis-soft);
  letter-spacing: 0.04em;
  text-transform: uppercase;
  white-space: nowrap;
  line-height: 1.1;
}

.section-line {
  flex: 1;
  height: 1px;
  background: var(--xy-border);
  margin-left: var(--xy-space-xs);
  opacity: 0.8;
}

.section-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  box-shadow: inset 0 1px 0 var(--xy-surface-sheen-soft);
  transition:
    transform 180ms var(--server-form-ease-smooth),
    border-color 180ms var(--server-form-ease-smooth),
    background-color 180ms var(--server-form-ease-smooth);
}

.section-header:hover .section-icon {
  transform: translateY(-1px);
}

.section-icon--accent {
  color: var(--xy-accent);
}

.section-icon--primary {
  color: var(--xy-primary);
}

.section-icon--success {
  color: var(--xy-success);
}

.section-icon--warning {
  color: var(--xy-warning);
}

.section-icon--muted {
  color: var(--xy-text-muted);
}

.deployment-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
}

.deployment-review {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.deployment-review-heading {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
}

.deployment-review-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.deployment-review-title {
  font-size: 0.88rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--xy-text-emphasis-strong);
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.deployment-review-subtitle {
  font-size: 0.82rem;
  line-height: 1.45;
}

.deployment-ready {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--xy-success-border-soft);
  border-radius: 10px;
  background:
    linear-gradient(180deg, var(--xy-success-bg-soft), transparent 65%),
    var(--xy-surface-raised-soft);
  transform-origin: top left;
}

.deployment-ready-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: 1px solid var(--xy-success-border-softer);
  color: var(--xy-success);
  background: var(--xy-success-bg-soft);
  flex-shrink: 0;
}

.deployment-ready-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.deployment-ready-label {
  font-size: 0.76rem;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--xy-success-text-soft);
}

.deployment-ready-value {
  color: var(--xy-text-primary);
  font-size: 0.94rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.deployment-review-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  flex-shrink: 0;
}

.deployment-review-dot {
  background: var(--xy-warning);
  box-shadow: 0 0 10px color-mix(in srgb, var(--xy-warning) 26%, transparent);
}

.deployment-summary-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  background:
    linear-gradient(180deg, var(--xy-surface-overlay-faint), transparent),
    var(--xy-surface-raised-subtle);
  border: 1px solid var(--xy-border);
  border-radius: 9px;
  transition:
    transform 180ms var(--server-form-ease-smooth),
    border-color 180ms var(--server-form-ease-smooth),
    background-color 180ms var(--server-form-ease-smooth);
}

.deployment-summary-item--warning {
  border-color: var(--xy-warning-border-soft);
}

.deployment-summary-item:hover {
  transform: translateY(-1px);
}

.deployment-summary-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--xy-surface-overlay-soft);
  border: 1px solid var(--xy-border);
  color: var(--xy-text-secondary);
  flex-shrink: 0;
}

.deployment-summary-icon.is-warning {
  color: var(--xy-warning);
  border-color: var(--xy-warning-border-soft);
}

.deployment-summary-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.deployment-summary-label {
  font-size: 0.7rem;
  letter-spacing: 0.07em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.deployment-summary-value {
  color: var(--xy-text-primary);
  font-size: 0.96rem;
  font-weight: 600;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

:deep(.server-form-shell .q-field--outlined .q-field__control) {
  background: var(--xy-surface-0);
  border-radius: 8px;
  transition:
    transform 180ms var(--server-form-ease-smooth),
    box-shadow 180ms var(--server-form-ease-smooth),
    background-color 180ms var(--server-form-ease-smooth);
}

:deep(.server-form-shell .q-field:hover .q-field__control),
:deep(.server-form-shell .q-field--focused .q-field__control) {
  transform: translateY(-1px);
  box-shadow: 0 10px 24px rgba(0, 0, 0, 0.16);
}

:deep(.server-form-shell .q-field__native),
:deep(.server-form-shell .q-field__input) {
  color: var(--xy-text-primary);
}

:deep(.server-form-shell .q-field__label) {
  color: var(--xy-field-label-color);
}

:deep(.server-form-shell .q-field__bottom) {
  color: var(--xy-field-helper-color);
  font-size: 0.8rem;
  line-height: 1.45;
}

.deployment-state-enter-active,
.deployment-state-leave-active {
  transition:
    opacity 220ms var(--server-form-ease-out),
    transform 220ms var(--server-form-ease-out);
}

.deployment-state-enter-from,
.deployment-state-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

@keyframes section-rise {
  from {
    opacity: 0;
    transform: translateY(14px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes save-ready-breathe {
  0%,
  100% {
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--xy-success) 22%, transparent);
  }

  50% {
    box-shadow:
      0 0 0 1px color-mix(in srgb, var(--xy-success) 32%, transparent),
      0 10px 24px color-mix(in srgb, var(--xy-success) 14%, transparent);
  }
}

@media (max-width: 720px) {
  .server-form-header {
    flex-direction: column;
    align-items: stretch;
    gap: var(--xy-space-md);
    padding: var(--xy-space-md);
  }

  .server-form-header-actions {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--xy-space-sm);
    width: 100%;
    padding-top: 0;
  }

  .server-form-header-actions :deep(.q-btn) {
    min-height: 44px;
  }

  .server-form-body {
    padding: var(--xy-space-sm) var(--xy-space-md) var(--xy-space-md);
  }

  .server-form-guidance {
    margin-bottom: var(--xy-space-md);
    font-size: 0.78rem;
  }

  .form-section {
    padding: var(--xy-space-md) 0;
  }

  .form-section--summary {
    margin-top: var(--xy-space-sm);
    padding-top: var(--xy-space-lg);
  }

  .section-header {
    margin-bottom: var(--xy-space-md);
  }

  .deployment-ready,
  .deployment-summary-item {
    padding: 12px;
  }

  .deployment-summary {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 520px) {
  .server-form-header {
    top: 50px;
  }

  .server-form-breadcrumbs {
    flex-wrap: wrap;
    row-gap: 2px;
  }

  .server-form-title {
    font-size: 1.18rem;
  }

  .server-form-subtitle {
    max-width: none;
    font-size: 0.84rem;
    line-height: 1.5;
  }

  .section-title,
  .deployment-review-title {
    font-size: 0.8rem;
    letter-spacing: 0.035em;
  }

  .deployment-review-subtitle,
  .deployment-ready-value {
    font-size: 0.84rem;
  }
}
</style>
