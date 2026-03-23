import type { Node, Status, VersionInfo } from '@/proto/shared_pb'
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
  versionInfo?: VersionInfo
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
        versionInfo: localServer.versionInfo,
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
    }

    if (!seenCompositeIDs.has(row.compositeId)) {
      seenCompositeIDs.add(row.compositeId)
      dedupedRows.push(row)
    }
  }

  return dedupedRows
}

export function extractRemoteNodeIDs(nodes: Node[]): Set<string> {
  const remoteNodeIDs = new Set<string>()
  for (const node of nodes) {
    if (!node.local) {
      remoteNodeIDs.add(node.id)
    }
  }
  return remoteNodeIDs
}

export function filterRowsByRemoteNodeIDs(
  rows: DisplayRow[],
  remoteNodeIDs: Set<string>,
): DisplayRow[] {
  const filteredRows: DisplayRow[] = []
  const seenCompositeIDs = new Set<string>()

  for (const row of rows) {
    if (seenCompositeIDs.has(row.compositeId)) {
      continue
    }
    if (row.isLocal || remoteNodeIDs.has(row.sourceNodeId)) {
      filteredRows.push({ ...row })
      seenCompositeIDs.add(row.compositeId)
    }
  }

  return filteredRows
}
