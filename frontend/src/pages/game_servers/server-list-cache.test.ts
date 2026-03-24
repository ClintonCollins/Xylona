import { describe, expect, it } from 'vitest'

import type { Node } from '@/proto/shared_pb'
import { Status, VersionStatus } from '@/proto/shared_pb'
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
          version: '1.20.4',
          versionInfo: {
            status: VersionStatus.CHECKED,
            installedVersion: '1.0.0',
            latestVersion: '1.1.0',
            updateAvailable: true,
            lastCheckTime: BigInt(0),
            trackerType: 'dummy',
          },
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
          version: '0.217.46',
          versionInfo: {
            status: VersionStatus.CHECKED,
            installedVersion: '0.217.46',
            latestVersion: '0.218.15',
            updateAvailable: true,
            lastCheckTime: BigInt(0),
            trackerType: 'steam',
          },
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
          version: '0.217.46',
          versionInfo: {
            status: VersionStatus.CHECKED,
            installedVersion: '0.217.46',
            latestVersion: '0.218.15',
            updateAvailable: true,
            lastCheckTime: BigInt(0),
            trackerType: 'steam',
          },
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
      version: '1.20.4',
    })
    expect(rows[0].versionInfo).toMatchObject({
      status: VersionStatus.CHECKED,
      installedVersion: '1.0.0',
      updateAvailable: true,
    })
    expect(rows[1]).toMatchObject({
      compositeId: 'node-remote/remote-1',
      id: 'remote-1',
      isLocal: false,
      nodeName: 'Remote Node',
      isStale: true,
      sourceNodeId: 'node-remote',
      version: '0.217.46',
    })
    expect(rows[1].versionInfo).toMatchObject({
      status: VersionStatus.CHECKED,
      installedVersion: '0.217.46',
      latestVersion: '0.218.15',
      updateAvailable: true,
    })
  })

  it('falls back to remote node id and host when source node id and name are missing', () => {
    const aggregatedServers = [
      {
        isLocal: false,
        remoteServer: {
          sourceNodeId: '',
          nodeId: 'node-remote-fallback',
          remoteServerId: 'remote-2',
          displayName: 'Remote Fallback',
          gameName: 'Terraria',
          status: Status.UNKNOWN,
          nodeName: '',
          nodeHost: 'https://fallback.example.com',
          version: '1.0.0',
          isStale: false,
        },
      },
    ] as unknown as AggregatedGameServer[]

    const rows = buildDisplayRows(aggregatedServers, new Map())
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({
      compositeId: 'node-remote-fallback/remote-2',
      sourceNodeId: 'node-remote-fallback',
      nodeName: 'https://fallback.example.com',
    })
  })

  it('ignores entries without matching local/remote payloads', () => {
    const aggregatedServers = [
      { isLocal: true, remoteServer: undefined, localServer: undefined },
      { isLocal: false, remoteServer: undefined, localServer: undefined },
    ] as unknown as AggregatedGameServer[]

    const rows = buildDisplayRows(aggregatedServers, new Map())
    expect(rows).toEqual([])
  })
})

describe('extractRemoteNodeIDs', () => {
  it('returns only non-local node ids', () => {
    const remoteNodeIDs = extractRemoteNodeIDs([
      { id: 'node-local', local: true },
      { id: 'node-remote-a', local: false },
      { id: 'node-remote-b', local: false },
    ] as unknown as Node[])

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
        version: '1.20.4',
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
        version: '0.217.46',
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
        version: '1.0.0',
      },
    ]

    const filteredRows = filterRowsByRemoteNodeIDs(rows, new Set(['node-remote-a']))
    expect(filteredRows).toHaveLength(2)
    expect(filteredRows.map((row) => row.compositeId)).toEqual([
      'local/local-1',
      'node-remote-a/remote-1',
    ])
  })

  it('deduplicates rows by composite id while preserving first occurrence', () => {
    const rows: DisplayRow[] = [
      {
        compositeId: 'node-remote-a/remote-1',
        id: 'remote-1',
        isLocal: false,
        displayName: 'First Value',
        gameName: 'Valheim',
        userName: '',
        statusEnum: Status.OFFLINE,
        nodeName: 'Remote A',
        isStale: false,
        sourceNodeId: 'node-remote-a',
        version: '0.217.46',
      },
      {
        compositeId: 'node-remote-a/remote-1',
        id: 'remote-1',
        isLocal: false,
        displayName: 'Second Value',
        gameName: 'Valheim',
        userName: '',
        statusEnum: Status.ONLINE,
        nodeName: 'Remote A',
        isStale: true,
        sourceNodeId: 'node-remote-a',
        version: '0.217.46',
      },
    ]

    const filteredRows = filterRowsByRemoteNodeIDs(rows, new Set(['node-remote-a']))
    expect(filteredRows).toHaveLength(1)
    expect(filteredRows[0].displayName).toBe('First Value')
  })
})
