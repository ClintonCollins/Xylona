import { describe, expect, it } from 'vitest'

import type { Node } from '@/proto/shared_pb'
import { Status, VersionStatus } from '@/proto/shared_pb'
import type { AggregatedGameServer } from '@/proto/xylona_pb'
import { buildDisplayRows } from './server-list-cache'

describe('buildDisplayRows', () => {
  it('uses configured capacity instead of the game player limit as fallback', () => {
    const aggregatedServers = [
      {
        isLocal: true,
        localServer: {
          id: 'local-1',
          name: 'Local One',
          status: Status.ONLINE,
          setMaxPlayers: 24n,
          maxPlayers: 100n,
          currentPlayerCount: 0n,
        },
      },
      {
        isLocal: false,
        remoteServer: {
          sourceNodeId: 'node-remote',
          remoteServerId: 'remote-1',
          displayName: 'Remote One',
          status: Status.ONLINE,
          currentPlayers: 0n,
          maxPlayers: 24n,
        },
      },
    ] as unknown as AggregatedGameServer[]

    const rows = buildDisplayRows(aggregatedServers, new Map())

    expect(rows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: 'local-1', currentPlayers: 0, maxPlayers: 24 }),
        expect.objectContaining({ id: 'remote-1', currentPlayers: 0, maxPlayers: 24 }),
      ]),
    )
  })

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
    expect(rows[0]?.versionInfo).toMatchObject({
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
    expect(rows[1]?.versionInfo).toMatchObject({
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
