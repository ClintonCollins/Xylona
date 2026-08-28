import { Status, type Node, type VersionInfo } from '@/proto/shared_pb'
import type { AggregatedGameServer } from '@/proto/xylona_pb'

export interface DisplayRow {
  compositeId: string
  id: string
  isLocal: boolean
  displayName: string
  gameName: string
  userName: string
  statusEnum: Status
  nodeName: string
  isStale: boolean
  sourceNodeId: string
  version: string
  versionInfo?: VersionInfo
  effectivePermissions?: string[]
  canUpdate?: boolean
  currentPlayers?: number
  maxPlayers?: number
  cpuPercent?: number | null
  memoryBytes?: number | null
  memoryPercent?: number | null
}

export function buildDisplayRows(
  aggregatedServers: AggregatedGameServer[],
  nodesByID: Map<string, Node>,
): DisplayRow[] {
  const dedupedRows: DisplayRow[] = []
  const seenCompositeIDs = new Set<string>()

  for (const server of aggregatedServers) {
    if (server.isLocal && server.localServer) {
      const localServer = server.localServer
      const nodeName =
        nodesByID.get(localServer.nodeId)?.name ||
        localServer.nodeName ||
        localServer.nodeHost ||
        'Local'
      const row: DisplayRow = {
        compositeId: 'local/' + localServer.id,
        id: localServer.id,
        isLocal: true,
        displayName: localServer.name,
        gameName: localServer.gameName,
        userName: localServer.userName,
        statusEnum: localServer.status,
        nodeName,
        isStale: false,
        sourceNodeId: '',
        version: localServer.version,
        versionInfo: localServer.versionInfo,
        effectivePermissions: [...(localServer.effectivePermissions ?? [])],
        canUpdate: localServer.resolvedHasUpdate,
        currentPlayers: Number(localServer.currentPlayerCount),
        maxPlayers: Number(localServer.setMaxPlayers || localServer.maxPlayers),
        cpuPercent: null,
        memoryBytes: null,
        memoryPercent: null,
      }

      if (!seenCompositeIDs.has(row.compositeId)) {
        seenCompositeIDs.add(row.compositeId)
        dedupedRows.push(row)
      }
      continue
    }

    if (server.isLocal || !server.remoteServer) {
      continue
    }

    const remoteServer = server.remoteServer
    const sourceNodeID = remoteServer.sourceNodeId || remoteServer.nodeId
    const nodeName =
      nodesByID.get(sourceNodeID)?.name ||
      remoteServer.nodeName ||
      remoteServer.nodeHost ||
      'Remote'
    const row: DisplayRow = {
      compositeId: sourceNodeID + '/' + remoteServer.remoteServerId,
      id: remoteServer.remoteServerId,
      isLocal: false,
      displayName: remoteServer.displayName,
      gameName: remoteServer.gameName,
      userName: '',
      statusEnum: remoteServer.status,
      nodeName,
      isStale: remoteServer.isStale,
      sourceNodeId: sourceNodeID,
      version: remoteServer.version,
      versionInfo: remoteServer.versionInfo,
      effectivePermissions: [...(remoteServer.effectivePermissions ?? [])],
      canUpdate: remoteServer.resolvedHasUpdate,
      currentPlayers: Number(remoteServer.currentPlayers),
      maxPlayers: Number(remoteServer.maxPlayers),
      cpuPercent: null,
      memoryBytes: null,
      memoryPercent: null,
    }

    if (!seenCompositeIDs.has(row.compositeId)) {
      seenCompositeIDs.add(row.compositeId)
      dedupedRows.push(row)
    }
  }

  return dedupedRows
}
