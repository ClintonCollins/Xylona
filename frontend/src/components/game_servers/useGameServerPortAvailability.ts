import { create } from '@bufbuild/protobuf'
import type { ComputedRef, Ref } from 'vue'
import { computed, onUnmounted, ref, watch } from 'vue'

import { GetXylonaClient } from '@/utils/shared'
import { suggestGameServerPorts } from '@/api/game-server-provisioning'
import type { Game, GameServer } from '@/proto/shared_pb'
import { ListGameServersRequestSchema } from '@/proto/xylona_pb'

const PORT_CHECK_DEBOUNCE_MS = 300

export type PortAvailabilityState = 'idle' | 'checking' | 'available' | 'conflict' | 'unavailable'

interface PortAvailabilityEvaluationOptions {
  excludeServerId?: string
  existingServers: GameServer[]
  nodeId: string
  ipAddress: string
  port: number
  queryPort: number
  selectedGame?: Game
}

function isValidPort(value: number): boolean {
  return Number.isInteger(value) && value >= 1 && value <= 65535
}

function buildPortLabel(port: number, queryPort: number): string {
  if (!isValidPort(queryPort) || port === queryPort) {
    return `${port}`
  }

  return `${port} / query ${queryPort}`
}

export function evaluateGameServerPortAvailability(options: PortAvailabilityEvaluationOptions): {
  message: string
  state: 'idle' | 'available' | 'conflict'
} {
  const normalizedNodeID = options.nodeId.trim()
  const normalizedIP = options.ipAddress.trim()
  if (normalizedNodeID.length === 0 || normalizedIP.length === 0 || !isValidPort(options.port)) {
    return {
      state: 'idle',
      message: '',
    }
  }

  const nodeServers = options.existingServers.filter(
    (server) =>
      server.id !== options.excludeServerId && (server.nodeId?.trim() ?? '') === normalizedNodeID,
  )
  const sameIPServers = nodeServers.filter((server) => server.ip?.address?.trim() === normalizedIP)
  const samePortNodeServer = nodeServers.find((server) => {
    const serverPort = Number(server.port ?? 0n)
    return isValidPort(serverPort) && serverPort === options.port
  })

  if (options.selectedGame?.bindsToAllIps && samePortNodeServer) {
    return {
      state: 'conflict',
      message: `${options.selectedGame.name || 'This game'} binds to all IPs, but port ${options.port} is already in use by ${samePortNodeServer.name || 'another server'} on this node.`,
    }
  }

  const bindAllServer = nodeServers.find((server) => {
    const serverPort = Number(server.port ?? 0n)
    return server.game?.bindsToAllIps && isValidPort(serverPort) && serverPort === options.port
  })
  if (bindAllServer) {
    return {
      state: 'conflict',
      message: `Port ${options.port} is already reserved by ${bindAllServer.name || 'another server'} because that game binds to all IPs on this node.`,
    }
  }

  for (const server of sameIPServers) {
    const serverPort = Number(server.port ?? 0n)
    if (isValidPort(serverPort) && serverPort === options.port) {
      return {
        state: 'conflict',
        message: `Port ${options.port} is already in use on ${normalizedIP} by ${server.name || 'another server'}.`,
      }
    }
  }

  return {
    state: 'available',
    message: `${normalizedIP}:${buildPortLabel(options.port, options.queryPort)} is available.`,
  }
}

export function useGameServerPortAvailability(options: {
  enabled: ComputedRef<boolean> | Ref<boolean>
  gameServer: Ref<GameServer>
  selectedGame: ComputedRef<Game | undefined> | Ref<Game | undefined>
}) {
  const portAvailabilityState = ref<PortAvailabilityState>('idle')
  const portAvailabilityMessage = ref('')
  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let requestToken = 0

  const currentRequest = computed(() => ({
    nodeId: options.gameServer.value.nodeId?.trim() ?? '',
    ipAddress: options.gameServer.value.ip?.address?.trim() ?? '',
    port: Number(options.gameServer.value.port ?? 0n),
    queryPort: Number(options.gameServer.value.queryPort ?? 0n),
    selectedGame: options.selectedGame.value,
  }))

  const canCheckAvailability = computed(
    () =>
      options.enabled.value &&
      currentRequest.value.nodeId.length > 0 &&
      currentRequest.value.ipAddress.length > 0 &&
      isValidPort(currentRequest.value.port),
  )

  function clearDebounceTimer() {
    if (debounceTimer !== undefined) {
      clearTimeout(debounceTimer)
      debounceTimer = undefined
    }
  }

  function resetState() {
    portAvailabilityState.value = 'idle'
    portAvailabilityMessage.value = ''
  }

  async function runAvailabilityCheck(): Promise<boolean> {
    clearDebounceTimer()

    if (!canCheckAvailability.value) {
      requestToken++
      resetState()
      return true
    }

    const currentToken = ++requestToken
    portAvailabilityState.value = 'checking'
    portAvailabilityMessage.value = `Checking ${currentRequest.value.ipAddress}:${buildPortLabel(currentRequest.value.port, currentRequest.value.queryPort)}...`

    try {
      const response = await GetXylonaClient().listGameServers(
        create(ListGameServersRequestSchema, {}),
      )
      if (currentToken !== requestToken) {
        return true
      }

      const evaluation = evaluateGameServerPortAvailability({
        existingServers: response.gameServers,
        nodeId: currentRequest.value.nodeId,
        ipAddress: currentRequest.value.ipAddress,
        port: currentRequest.value.port,
        queryPort: currentRequest.value.queryPort,
        selectedGame: currentRequest.value.selectedGame,
      })

      portAvailabilityState.value = evaluation.state
      portAvailabilityMessage.value = evaluation.message

      return evaluation.state !== 'conflict'
    } catch (error) {
      if (currentToken !== requestToken) {
        return true
      }

      console.error(error)
      portAvailabilityState.value = 'unavailable'
      portAvailabilityMessage.value =
        'Live port check unavailable. Save will still verify on the server.'

      return true
    }
  }

  function scheduleAvailabilityCheck() {
    clearDebounceTimer()

    if (!canCheckAvailability.value) {
      requestToken++
      resetState()
      return
    }

    debounceTimer = setTimeout(() => {
      void runAvailabilityCheck()
    }, PORT_CHECK_DEBOUNCE_MS)
  }

  // The pair the form last filled in by itself, and the port it started from.
  const autoPorts = ref<{ from: bigint; port: bigint; queryPort: bigint } | null>(null)
  let suggestionToken = 0

  function formHasPorts(pair: { port: bigint; queryPort: bigint }): boolean {
    return (
      options.gameServer.value.port === pair.port &&
      options.gameServer.value.queryPort === pair.queryPort
    )
  }

  // Fill in the first free pair at or after `from`, using the controller's own check.
  async function applyNextFreePorts(from: { port: bigint; queryPort: bigint }): Promise<void> {
    const { nodeId, ipAddress } = currentRequest.value
    const gameId = options.selectedGame.value?.id ?? ''
    if (!gameId || !nodeId || !ipAddress || from.port <= 0n) {
      return
    }

    const token = ++suggestionToken
    const requested = {
      port: options.gameServer.value.port,
      queryPort: options.gameServer.value.queryPort,
    }
    try {
      const suggested = await suggestGameServerPorts({ nodeId, ipAddress, gameId, ...from })
      // Drop stale answers and never overwrite ports typed while this was in flight.
      if (token !== suggestionToken || !formHasPorts(requested)) {
        return
      }
      options.gameServer.value.port = suggested.port
      options.gameServer.value.queryPort = suggested.queryPort
      autoPorts.value = { from: from.port, ...suggested }
    } catch (error) {
      // The live check below still flags a taken port.
      console.error(error)
    }
  }

  const portSuggestionNote = computed(() => {
    const filled = autoPorts.value
    if (filled === null || filled.port === filled.from || !formHasPorts(filled)) {
      return ''
    }
    return `Port ${filled.from} is already used on this node, so the next free port was filled in.`
  })

  // Re-pick ports the form filled in when the game, node or IP changes; typed ports stay.
  // Separate sources, so filling the ports in doesn't re-trigger this watcher.
  watch(
    [
      () => options.enabled.value,
      () => options.selectedGame.value?.id ?? '',
      () => currentRequest.value.nodeId,
      () => currentRequest.value.ipAddress,
    ],
    () => {
      const game = options.selectedGame.value
      if (!options.enabled.value || game === undefined) {
        return
      }
      const defaults = { port: game.defaultPort, queryPort: game.defaultQueryPort }
      const filled = autoPorts.value
      if (formHasPorts(defaults) || (filled !== null && formHasPorts(filled))) {
        void applyNextFreePorts(defaults)
      }
    },
    { immediate: true },
  )

  watch(
    () => [
      options.enabled.value,
      currentRequest.value.selectedGame?.id ?? '',
      currentRequest.value.selectedGame?.bindsToAllIps ?? false,
      currentRequest.value.nodeId,
      currentRequest.value.ipAddress,
      currentRequest.value.port,
      currentRequest.value.queryPort,
    ],
    () => {
      scheduleAvailabilityCheck()
    },
    {
      immediate: true,
    },
  )

  onUnmounted(() => {
    requestToken++
    suggestionToken++
    clearDebounceTimer()
  })

  return {
    ensurePortAvailabilityBeforeSave: runAvailabilityCheck,
    portSuggestionNote,
    fillNextFreePorts: () =>
      applyNextFreePorts({
        port: options.gameServer.value.port,
        queryPort: options.gameServer.value.queryPort,
      }),
    portAvailabilityBlocking: computed(() => portAvailabilityState.value === 'conflict'),
    portAvailabilityChecking: computed(() => portAvailabilityState.value === 'checking'),
    portAvailabilityMessage: computed(() => portAvailabilityMessage.value),
    portAvailabilityState: computed(() => portAvailabilityState.value),
    portAvailabilityVisible: computed(() => portAvailabilityState.value !== 'idle'),
  }
}
