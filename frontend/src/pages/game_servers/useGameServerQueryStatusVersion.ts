import { create } from '@bufbuild/protobuf'
import { onUnmounted, ref, type Ref } from 'vue'
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

interface UseGameServerQueryStatusVersionOptions {
  gameServer: Ref<GameServer>
  gameServerId: Ref<string>
}

export function useGameServerQueryStatusVersion({
  gameServer,
  gameServerId,
}: UseGameServerQueryStatusVersionOptions) {
  const $q = useQuasar()
  const currentPlayerCount = ref(0)
  const maxPlayerCount = ref(0)
  let lifecycleStarted = false
  let lifecycleUnmounted = false

  function applyQueryInfo(queryInfo: ServerQuery) {
    switch (queryInfo.type) {
      case ServerQuery_Type.Minecraft: {
        const minecraftQuery = queryInfo.minecraft
        if (minecraftQuery === undefined) {
          return
        }

        currentPlayerCount.value = minecraftQuery.numberOfPlayers
        maxPlayerCount.value = minecraftQuery.maxPlayers
        break
      }
      case ServerQuery_Type.Source: {
        const sourceQuery = queryInfo.source
        if (sourceQuery === undefined) {
          return
        }

        currentPlayerCount.value = sourceQuery.players
        maxPlayerCount.value = sourceQuery.maxPlayers
        break
      }
      case ServerQuery_Type.Palworld: {
        const palworldQuery = queryInfo.palworld
        if (palworldQuery === undefined) {
          return
        }

        currentPlayerCount.value = palworldQuery.players
        maxPlayerCount.value = palworldQuery.maxPlayers
        break
      }
    }
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
      console.error(error)
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
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
    currentPlayerCount,
    maxPlayerCount,
    onServerQueryInfo,
    onServerStatusUpdate,
    onServerVersionUpdate,
    queryGameServer,
    startQueryStatusVersionLifecycle,
    stopQueryStatusVersionLifecycle,
  }
}
