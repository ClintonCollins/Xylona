import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { computed, ref } from 'vue'
import type { QForm } from 'quasar'
import { useQuasar } from 'quasar'

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
import { Game, GameServerSchema, IP, Node } from '@/proto/shared_pb'
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

export interface GameServerFormStateOptions {
  existingGameServerId?: string
  loadProvisioningOptions: boolean
}

export function useGameServerFormState(options: GameServerFormStateOptions) {
  const $q = useQuasar()

  const gameServer = ref(create(GameServerSchema, {}))
  const availableGames = ref<Array<Record<string, string>>>([])
  const availableUsers = ref<Array<Record<string, string>>>([])
  const availableIPs = ref<Array<IP>>([])
  const gamesMap = ref(new Map<string, Game>())
  const nodes = ref<Array<Node>>([])

  const loading = ref(true)
  const formSubmitting = ref(false)
  const formRef = ref<QForm | null>(null)

  const isEditing = computed(() => options.existingGameServerId !== undefined)
  const isMinecraftGame = computed(() => gameServer.value.gameId === 'minecraft')
  const trimmedServerName = computed(() => gameServer.value.name?.trim() ?? '')
  const selectedGameName = computed(
    () =>
      gamesMap.value.get(gameServer.value.gameId)?.name ??
      gameServer.value.gameName ??
      'Choose a game',
  )
  const selectedGame = computed(() => gamesMap.value.get(gameServer.value.gameId))
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

  const maxMemoryStateMessage = computed(() =>
    isMinecraftGame.value ? (describeMinecraftMemoryState(maxMemoryModel.value) ?? '') : '',
  )
  const showMaxMemoryStateError = computed(
    () => isEditing.value && isMinecraftGame.value && maxMemoryStateMessage.value.length > 0,
  )

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

  const provisioningConnection = computed(() =>
    queryPortModel.value > 0 && queryPortModel.value !== portModel.value
      ? `${selectedIPLabel.value}:${portModel.value || 0} / query ${queryPortModel.value}`
      : `${selectedIPLabel.value}:${portModel.value || 0}`,
  )

  const provisioningCapacity = computed(
    () => `${maxPlayersModel.value || 0} max / start ${setPlayersModel.value || 0}`,
  )
  const serverExecutableSummary = computed(() => {
    const executable = gameServer.value.serverExecutable?.trim() ?? ''
    if (executable.length > 0) {
      return executable
    }

    return 'Not set'
  })

  const serverNameRules = [
    (value: string | null | undefined) => validateRequiredText(value, 'Server Name'),
  ]
  const gameRules = [(value: string | null | undefined) => validateRequiredSelection(value, 'Game')]
  const ownerRules = [
    (value: string | null | undefined) => validateRequiredSelection(value, 'Owner'),
  ]
  const nodeRules = [(value: string | null | undefined) => validateRequiredSelection(value, 'Node')]
  const ipRules = [(value: IP | null | undefined) => validateRequiredValue(value, 'IP Address')]
  const portRules = [(value: number | string | bigint | null | undefined) => validatePort(value)]
  const queryPortRules = [
    (value: number | string | bigint | null | undefined) => validatePort(value),
  ]
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

  async function initialize() {
    loading.value = true

    try {
      if (options.existingGameServerId) {
        await getGameServerDetails()
      }

      if (options.loadProvisioningOptions) {
        await Promise.all([getGames(), getNodes(), getUsers(), getIPs()])
      }
    } finally {
      loading.value = false
    }
  }

  function onGameSelected(gameId: string) {
    const selectedGame = gamesMap.value.get(gameId)
    if (!selectedGame) {
      return
    }

    applyGameDefaults(selectedGame)
  }

  async function validateBeforeSave(invalidMessage: string) {
    if (!isMinecraftGame.value) {
      gameServer.value.maxMemoryMb = 0n
    }

    const formValid = (await formRef.value?.validate()) ?? true
    if (!formValid) {
      $q.notify({
        type: 'warning',
        position: 'top',
        caption: invalidMessage,
        icon: 'report_problem',
      })
      return false
    }

    return true
  }

  function resetSubmissionState() {
    formSubmitting.value = false
  }

  function startSubmitting() {
    formSubmitting.value = true
  }

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

  async function getGameServerDetails() {
    const existingGameServerId = options.existingGameServerId
    if (!existingGameServerId) {
      return
    }

    const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})

    try {
      request.id = existingGameServerId
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
        caption:
          'Failed to load game server details: ' + ConnectErrorToString(ConnectError.from(e)),
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
      nodes.value = response.nodes.slice()

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
      console.error(unknownError)
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
      availableUsers.value = response.users.map((user) => ({
        label: user.userName,
        value: user.id,
      }))

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
      availableIPs.value = response.ips.slice()

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

  return {
    availableGames,
    availableIPs,
    availableUsers,
    deploymentReady,
    deploymentReadyText,
    deploymentWarningItems,
    formRef,
    formSubmitting,
    gameRules,
    gameServer,
    initialize,
    ipRules,
    isEditing,
    isMinecraftGame,
    loading,
    maxMemoryModel,
    maxMemoryRules,
    maxMemoryStateMessage,
    maxPlayersModel,
    maxPlayersRules,
    nodeRules,
    nodes,
    onGameSelected,
    ownerRules,
    portModel,
    portRules,
    provisioningCapacity,
    provisioningConnection,
    queryPortModel,
    queryPortRules,
    resetSubmissionState,
    selectedGame,
    selectedGameName,
    selectedNodeName,
    selectedOwnerName,
    serverExecutableSummary,
    serverNameRules,
    setPlayersModel,
    setPlayersRules,
    showMaxMemoryStateError,
    startSubmitting,
    validateBeforeSave,
  }
}
