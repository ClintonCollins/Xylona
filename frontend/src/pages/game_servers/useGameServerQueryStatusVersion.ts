import { create } from '@bufbuild/protobuf'
import { computed, onUnmounted, ref, type Ref } from 'vue'
import {
  type AllServersQueryInfo,
  type GameServer,
  ServerQuery_Type,
  type ServerQuery,
  Status,
  type VersionInfo,
} from '@/proto/shared_pb'
import {
  QueryGameServerRequestSchema,
  type QueryGameServerRequest,
  type QueryGameServerResponse,
} from '@/proto/xylona_pb'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'

import { websocketStateAuthoritative } from '@/utils/websocket-connection'

interface UseGameServerQueryStatusVersionOptions {
  gameServer: Ref<GameServer>
  gameServerId: Ref<string>
}

export interface QueryPlayerSnapshot {
  playerCount: number
  playerCapacity: number
  players: string[]
  playerListSupported: boolean
  /** False for the placeholder the controller stores when the game did not answer. */
  responded: boolean
}

export function queryInfoPlayerSnapshot(queryInfo: ServerQuery): QueryPlayerSnapshot | null {
  switch (queryInfo.type) {
    case ServerQuery_Type.Minecraft: {
      const minecraftQuery = queryInfo.minecraft
      if (minecraftQuery === undefined) return null
      return {
        playerCount: minecraftQuery.numberOfPlayers,
        playerCapacity: minecraftQuery.maxPlayers,
        players: [...minecraftQuery.playerList],
        playerListSupported: true,
        responded: minecraftQuery.responded,
      }
    }
    case ServerQuery_Type.Source: {
      const sourceQuery = queryInfo.source
      if (sourceQuery === undefined) return null
      return {
        playerCount: sourceQuery.players,
        playerCapacity: sourceQuery.maxPlayers,
        players: [...sourceQuery.playerList],
        playerListSupported: sourceQuery.playerListSupported,
        responded: sourceQuery.responded,
      }
    }
    case ServerQuery_Type.Palworld: {
      const palworldQuery = queryInfo.palworld
      if (palworldQuery === undefined) return null
      return {
        playerCount: palworldQuery.players,
        playerCapacity: palworldQuery.maxPlayers,
        players: [...palworldQuery.playerList],
        playerListSupported: true,
        responded: palworldQuery.responded,
      }
    }
    default:
      return null
  }
}

export function useGameServerQueryStatusVersion({
  gameServer,
  gameServerId,
}: UseGameServerQueryStatusVersionOptions) {
  const $q = useQuasar()
  const currentPlayerCount = ref(0)
  const maxPlayerCount = ref(0)
  const onlinePlayers = ref<string[]>([])
  const playerListSupported = ref(false)
  // The controller pushes query results only when they change, so the latest
  // one stays current until a new one (or a failed placeholder) replaces it.
  const queryResponded = ref(false)
  /** False when the game has no player query at all, so its count is never known. */
  const querySupported = ref(true)
  const queryFresh = computed(
    () =>
      gameServer.value.status === Status.ONLINE &&
      websocketStateAuthoritative.value &&
      queryResponded.value,
  )
  /** Players online, or null while the server is up but its count is unknown. */
  const playerCount = computed<number | null>(() => {
    if (gameServer.value.status !== Status.ONLINE) return 0
    return queryFresh.value ? currentPlayerCount.value : null
  })
  /** Why playerCount is null, for the players panel. */
  const unknownPlayersMessage = computed(() =>
    querySupported.value
      ? 'Player count and names unavailable. The game has not answered a player query.'
      : 'This game does not report its player count or names.',
  )
  let lifecycleStarted = false
  let lifecycleUnmounted = false

  function applyQueryInfo(queryInfo: ServerQuery) {
    querySupported.value = queryInfo.type !== ServerQuery_Type.Unknown
    const snapshot = queryInfoPlayerSnapshot(queryInfo)
    if (snapshot === null) {
      return
    }
    if (!snapshot.responded) {
      queryResponded.value = false
      currentPlayerCount.value = 0
      maxPlayerCount.value = snapshot.playerCapacity
      onlinePlayers.value = []
      playerListSupported.value = false
      return
    }

    queryResponded.value = true
    currentPlayerCount.value = snapshot.playerCount
    maxPlayerCount.value = snapshot.playerCapacity
    onlinePlayers.value = snapshot.players
    playerListSupported.value = snapshot.playerListSupported
  }

  async function queryGameServer() {
    const request: QueryGameServerRequest = create(QueryGameServerRequestSchema, {})
    try {
      request.serverId = gameServerId.value
      const response: QueryGameServerResponse = await GetXylonaClient().queryGameServer(request)
      if (response.queryInfo !== undefined) {
        applyQueryInfo(response.queryInfo)
      }
    } catch (error) {
      queryResponded.value = false
      console.error(error)
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption: 'Failed to query game server: ' + ConnectErrorToString(ConnectError.from(error)),
        icon: 'report_problem',
      })
    }
  }

  function onServerQueryInfo(allServersQueryInfo: AllServersQueryInfo) {
    const queryInfo = allServersQueryInfo.servers[gameServerId.value]
    if (queryInfo === undefined) {
      return
    }

    applyQueryInfo(queryInfo)
  }

  function onServerStatusUpdate(serverID: string, _serverName: string, serverStatus: Status) {
    if (serverID !== gameServerId.value) {
      return
    }

    gameServer.value.status = serverStatus
  }

  function onServerVersionUpdate(serverID: string, version: string, versionInfo?: VersionInfo) {
    if (serverID !== gameServerId.value) {
      return
    }

    gameServer.value.version = version
    gameServer.value.versionInfo = versionInfo
  }

  function startQueryStatusVersionLifecycle() {
    if (lifecycleUnmounted || lifecycleStarted) {
      return
    }

    lifecycleStarted = true
    XylonaEventBus.on('gameServersQueryInfo', onServerQueryInfo)
    XylonaEventBus.on('gameServerStatus', onServerStatusUpdate)
    XylonaEventBus.on('gameServerVersion', onServerVersionUpdate)
  }

  function stopQueryStatusVersionLifecycle() {
    if (!lifecycleStarted) {
      return
    }

    lifecycleStarted = false
    XylonaEventBus.off('gameServersQueryInfo', onServerQueryInfo)
    XylonaEventBus.off('gameServerStatus', onServerStatusUpdate)
    XylonaEventBus.off('gameServerVersion', onServerVersionUpdate)
  }

  onUnmounted(() => {
    lifecycleUnmounted = true
    stopQueryStatusVersionLifecycle()
  })

  return {
    queryFresh,
    playerCount,
    currentPlayerCount,
    maxPlayerCount,
    onlinePlayers,
    onServerQueryInfo,
    onServerStatusUpdate,
    onServerVersionUpdate,
    playerListSupported,
    unknownPlayersMessage,
    queryGameServer,
    startQueryStatusVersionLifecycle,
    stopQueryStatusVersionLifecycle,
  }
}
