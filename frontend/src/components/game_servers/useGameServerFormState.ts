import { create } from '@bufbuild/protobuf'
import { computed, ref, watch } from 'vue'
import type { QForm } from 'quasar'
import { useQuasar } from 'quasar'

import { buildXylonaErrorNotification, connectErrorMessage } from '@/api/connect-errors'
import {
  getGameServer,
  listGames,
  listIPs,
  listNodes,
  listUsers,
  type ProvisioningOption,
} from '@/api/game-server-provisioning'
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

export interface GameServerFormStateOptions {
  existingGameServerId?: string
  loadProvisioningOptions: boolean
}

export function useGameServerFormState(options: GameServerFormStateOptions) {
  const $q = useQuasar()

  const gameServer = ref(create(GameServerSchema, {}))
  const availableGames = ref<ProvisioningOption[]>([])
  const availableUsers = ref<ProvisioningOption[]>([])
  const availableIPs = ref<Array<IP>>([])
  const gamesMap = ref(new Map<string, Game>())
  const nodes = ref<Array<Node>>([])

  const loading = ref(true)
  const formSubmitting = ref(false)
  const formRef = ref<QForm | null>(null)
  let allowNodeIPRefresh = false
  let ipRequestSequence = 0

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

  const autoRestartMaxRetriesModel = computed({
    get: () => Number(gameServer.value.autoRestartMaxRetries ?? 3n),
    set: (value: number | string | null | undefined) => {
      gameServer.value.autoRestartMaxRetries = toBigInt(value)
    },
  })

  const autoRestartCooldownModel = computed({
    get: () => Number(gameServer.value.autoRestartCooldownSeconds ?? 30n),
    set: (value: number | string | null | undefined) => {
      gameServer.value.autoRestartCooldownSeconds = toBigInt(value)
    },
  })

  const autoRestartMaxRetriesRules = [
    (value: number | string | bigint | null | undefined) => {
      const num = Number(value)
      if (!Number.isFinite(num) || num < 1) return 'Minimum 1 retry'
      if (num > 20) return 'Maximum 20 retries'
      return true
    },
  ]

  const autoRestartCooldownRules = [
    (value: number | string | bigint | null | undefined) => {
      const num = Number(value)
      if (!Number.isFinite(num) || num < 10) return 'Minimum 10 seconds'
      if (num > 3600) return 'Maximum 3600 seconds (1 hour)'
      return true
    },
  ]

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
        await Promise.all([getGames(), getNodes(), getUsers()])
        await getIPs()
        allowNodeIPRefresh = true
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

    try {
      const existingGameServer = await getGameServer(existingGameServerId)
      if (existingGameServer === undefined) {
        return
      }

      gameServer.value = existingGameServer
    } catch (e) {
      console.error(e)
      $q.notify({
        ...buildXylonaErrorNotification(
          connectErrorMessage(e, 'Failed to load game server details'),
        ),
        icon: 'report_problem',
      })
    }
  }

  async function getGames() {
    try {
      const provisioningGames = await listGames()
      availableGames.value = provisioningGames.options
      gamesMap.value = new Map(provisioningGames.games.map((game) => [game.id, game]))

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

      const firstAvailableGame = availableGames.value[0]
      if (!firstAvailableGame) {
        return
      }

      const firstGame = gamesMap.value.get(firstAvailableGame.value)
      if (!firstGame) {
        return
      }

      applyGameDefaults(firstGame)
    } catch (e) {
      console.error(e)
      $q.notify({
        ...buildXylonaErrorNotification(connectErrorMessage(e, 'Failed to load games')),
        icon: 'report_problem',
      })
    }
  }

  async function getNodes() {
    try {
      nodes.value = await listNodes()

      if (gameServer.value.nodeId) {
        return
      }

      const localNode = nodes.value.find((node) => node.local)
      if (localNode) {
        gameServer.value.nodeId = localNode.id
        gameServer.value.nodeName = localNode.name
        return
      }

      const firstNode = nodes.value[0]
      if (firstNode) {
        gameServer.value.nodeId = firstNode.id
        gameServer.value.nodeName = firstNode.name
      }
    } catch (unknownError: unknown) {
      console.error(unknownError)
      $q.notify({
        ...buildXylonaErrorNotification(connectErrorMessage(unknownError, 'Failed to load nodes')),
        icon: 'report_problem',
      })
    }
  }

  async function getUsers() {
    try {
      availableUsers.value = await listUsers()

      if (availableUsers.value.length === 0 || gameServer.value.userId) {
        return
      }

      const firstUser = availableUsers.value[0]
      if (!firstUser) {
        return
      }

      gameServer.value.userId = firstUser.value
      gameServer.value.userName = firstUser.label
    } catch (e) {
      console.error(e)
      $q.notify({
        ...buildXylonaErrorNotification(connectErrorMessage(e, 'Failed to load users')),
        icon: 'report_problem',
      })
    }
  }

  async function getIPs() {
    const requestSequence = ++ipRequestSequence
    const nodeId = gameServer.value.nodeId?.trim() ?? ''
    const isStaleRequest = () =>
      requestSequence !== ipRequestSequence || (gameServer.value.nodeId?.trim() ?? '') !== nodeId

    try {
      if (nodeId.length === 0) {
        if (isStaleRequest()) {
          return
        }
        availableIPs.value = []
        gameServer.value.ip = undefined
        return
      }

      const previousAddress = gameServer.value.ip?.address?.trim() ?? ''
      const nextIPs = await listIPs(nodeId)
      if (isStaleRequest()) {
        return
      }

      availableIPs.value = nextIPs

      const existingSelection = availableIPs.value.find((ip) => ip.address === previousAddress)
      if (existingSelection) {
        gameServer.value.ip = existingSelection
        return
      }

      const preferredExternalIP = availableIPs.value.find((ip) => ip.external)
      if (preferredExternalIP) {
        gameServer.value.ip = preferredExternalIP
        return
      }

      const firstAvailableIP = availableIPs.value[0]
      if (firstAvailableIP) {
        gameServer.value.ip = firstAvailableIP
        return
      }

      gameServer.value.ip = undefined
    } catch (e) {
      if (isStaleRequest()) {
        return
      }
      console.error(e)
      $q.notify({
        ...buildXylonaErrorNotification(connectErrorMessage(e, 'Failed to load IP addresses')),
        icon: 'report_problem',
      })
    }
  }

  watch(
    () => gameServer.value.nodeId,
    async (nodeId, previousNodeId) => {
      if (!allowNodeIPRefresh) {
        return
      }
      if ((nodeId ?? '') === (previousNodeId ?? '')) {
        return
      }
      await getIPs()
    },
  )

  return {
    autoRestartCooldownModel,
    autoRestartCooldownRules,
    autoRestartMaxRetriesModel,
    autoRestartMaxRetriesRules,
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
