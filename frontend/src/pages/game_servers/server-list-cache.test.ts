import { describe, expect, it } from 'vitest'

import type { Node } from '@/proto/shared_pb'
import { Status } from '@/proto/shared_pb'
import type { AggregatedGameServer } from '@/proto/xylona_pb'
import {
  buildDisplayRows,
  extractRemoteNodeIDs,
  filterRowsByRemoteNodeIDs,
  type DisplayRow,
} from './server-list-cache'

describe('buildDisplayRows', () => {
  it('builds deduplicated local and remote rows', () => {
    const aggregatedServers = [
      {
        isLocal: true,
        localServer: {
          id: 'local-1',
          name: 'Local One',
          gameName: 'Minecraft',
          userName: 'admin',
          status: Status.ONLINE,
          nodeId: 'node-local',
          nodeName: '',
          nodeHost: '',
        },
      },
      {
        isLocal: false,
        remoteServer: {
          sourceNodeId: 'node-remote',
          nodeId: 'node-remote',
          remoteServerId: 'remote-1',
          displayName: 'Remote One',
          gameName: 'Valheim',
          status: Status.OFFLINE,
          nodeName: 'Remote Node Name',
          nodeHost: 'https://remote.example.com',
          isStale: true,
        },
      },
      {
        isLocal: false,
        remoteServer: {
          sourceNodeId: 'node-remote',
          nodeId: 'node-remote',
          remoteServerId: 'remote-1',
          displayName: 'Remote One Duplicate',
          gameName: 'Valheim',
          status: Status.OFFLINE,
          nodeName: 'Remote Node Name',
          nodeHost: 'https://remote.example.com',
          isStale: true,
        },
      },
    ] as unknown as AggregatedGameServer[]

    const rows = buildDisplayRows(
      aggregatedServers,
      new Map([
        ['node-local', { id: 'node-local', name: 'Local Node', local: true }],
        ['node-remote', { id: 'node-remote', name: 'Remote Node', local: false }],
      ]) as unknown as Map<string, Node>,
    )

    expect(rows).toHaveLength(2)
    expect(rows[0]).toMatchObject({
      compositeId: 'local/local-1',
      id: 'local-1',
      isLocal: true,
      nodeName: 'Local Node',
    })
    expect(rows[1]).toMatchObject({
      compositeId: 'node-remote/remote-1',
      id: 'remote-1',
      isLocal: false,
      nodeName: 'Remote Node',
      isStale: true,
      sourceNodeId: 'node-remote',
    })
  })
})

describe('extractRemoteNodeIDs', () => {
  it('returns only non-local node ids', () => {
    const remoteNodeIDs = extractRemoteNodeIDs(
      [
        { id: 'node-local', local: true },
        { id: 'node-remote-a', local: false },
        { id: 'node-remote-b', local: false },
      ] as unknown as Node[],
    )

    expect(Array.from(remoteNodeIDs)).toEqual(['node-remote-a', 'node-remote-b'])
  })
})

describe('filterRowsByRemoteNodeIDs', () => {
  it('keeps local rows and remote rows with existing remote nodes only', () => {
    const rows: DisplayRow[] = [
      {
        compositeId: 'local/local-1',
        id: 'local-1',
        isLocal: true,
        displayName: 'Local One',
        gameName: 'Minecraft',
        userName: 'admin',
        statusEnum: Status.ONLINE,
        nodeName: 'Local Node',
        isStale: false,
        sourceNodeId: '',
      },
      {
        compositeId: 'node-remote-a/remote-1',
        id: 'remote-1',
        isLocal: false,
        displayName: 'Remote One',
        gameName: 'Valheim',
        userName: '',
        statusEnum: Status.OFFLINE,
        nodeName: 'Remote A',
        isStale: false,
        sourceNodeId: 'node-remote-a',
      },
      {
        compositeId: 'node-remote-missing/remote-2',
        id: 'remote-2',
        isLocal: false,
        displayName: 'Remote Two',
        gameName: 'Rust',
        userName: '',
        statusEnum: Status.UNKNOWN,
        nodeName: 'Remote Missing',
        isStale: true,
        sourceNodeId: 'node-remote-missing',
      },
    ]

    const filteredRows = filterRowsByRemoteNodeIDs(rows, new Set(['node-remote-a']))
    expect(filteredRows).toHaveLength(2)
    expect(filteredRows.map(row => row.compositeId)).toEqual([
      'local/local-1',
      'node-remote-a/remote-1',
    ])
  })
})
