import { create } from '@bufbuild/protobuf'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BackupProgressOperation, BackupProgressPhase, VersionStatus } from '@/proto/shared_pb'
import { GameServerFilesCompressionType } from '@/proto/gameserver_files_operations_pb'
import { AllServersMetrics, Message_Type, MessageSchema } from '@/proto/websocket_pb'

import {
  ArchiveTypeToExtension,
  ArchiveTypeToString,
  bytesToSize,
  dispatchWebsocketMessage,
  GetPathSeparator,
  GetRelativeFilePath,
  XylonaEventBus,
} from './shared'

describe('GetRelativeFilePath', () => {
  it.each([
    {
      name: 'joins Unix paths with /',
      root: '/home/server',
      segments: ['saves', 'world'],
      want: 'saves/world',
    },
    {
      name: 'joins Windows paths with backslash',
      root: 'C:\\server\\data',
      segments: ['saves', 'world'],
      want: 'saves\\world',
    },
    {
      name: 'filters empty segments',
      root: '/home/server',
      segments: ['', 'saves', '', 'world'],
      want: 'saves/world',
    },
    {
      name: 'returns empty string when all segments are empty',
      root: '/home/server',
      segments: ['', '', ''],
      want: '',
    },
    {
      name: 'returns single segment unchanged',
      root: '/home/server',
      segments: ['saves'],
      want: 'saves',
    },
    {
      name: 'filters undefined segments',
      root: '/home/server',
      segments: [undefined as unknown as string, 'saves'],
      want: 'saves',
    },
  ])('$name', ({ root, segments, want }) => {
    expect(GetRelativeFilePath(root, ...segments)).toBe(want)
  })
})

describe('GetPathSeparator', () => {
  it.each([
    ['Windows paths', 'C:\\Users\\test', '\\'],
    ['Unix paths', '/home/user/test', '/'],
    ['paths with no separator', 'filename.txt', '/'],
  ])('returns the separator for %s', (_name, path, want) => {
    expect(GetPathSeparator(path)).toBe(want)
  })
})

describe('bytesToSize', () => {
  it.each([
    [0, '0 Byte'],
    [500, '500 Bytes'],
    [1024, '1 KB'],
    [1048576, '1 MB'],
    [1073741824, '1 GB'],
  ])('formats %i bytes as %s', (bytes, want) => {
    expect(bytesToSize(bytes)).toBe(want)
  })
})

describe('ArchiveTypeToString', () => {
  it.each([
    [GameServerFilesCompressionType.ZIP, 'ZIP (.zip)'],
    [GameServerFilesCompressionType.GZIP, 'Gzip (.gz)'],
    [GameServerFilesCompressionType.BZIP2, 'Bzip2 (.bz2)'],
    [GameServerFilesCompressionType.ZST, 'Zstandard (.zst)'],
    [GameServerFilesCompressionType.XZ, 'XZ (.xz)'],
    [999 as GameServerFilesCompressionType, 'Unknown'],
  ])('maps %s to %s', (type, want) => {
    expect(ArchiveTypeToString(type)).toBe(want)
  })
})

describe('ArchiveTypeToExtension', () => {
  it.each([
    [GameServerFilesCompressionType.ZIP, '.zip'],
    [GameServerFilesCompressionType.GZIP, '.tar.gz'],
    [GameServerFilesCompressionType.BZIP2, '.tar.bz2'],
    [GameServerFilesCompressionType.ZST, '.tar.zst'],
    [GameServerFilesCompressionType.XZ, '.tar.xz'],
    [999 as GameServerFilesCompressionType, '.unknown'],
  ])('maps %s to %s', (type, want) => {
    expect(ArchiveTypeToExtension(type)).toBe(want)
  })
})

describe('XylonaEventBus remoteServerMetrics', () => {
  afterEach(() => {
    // Clean up all listeners to avoid leaking between tests.
    XylonaEventBus.off('remoteServerMetrics')
  })

  it('should emit per-server remoteServerMetrics when gameServerMetrics arrives', () => {
    // The WebSocket onmessage handler receives an AllServersMetrics message
    // containing a map of server_id -> GameServerMetrics. In addition to
    // emitting the bulk "gameServerMetrics" event, it should fan out
    // individual "remoteServerMetrics" events keyed by server ID so that
    // individual GameServerView components can subscribe to metrics for a
    // single server without processing the entire map.

    const perServerHandler = vi.fn()
    XylonaEventBus.on('remoteServerMetrics', perServerHandler)

    // Simulate what the onmessage handler currently does: emit gameServerMetrics
    // with the full AllServersMetrics payload. The new behavior should also
    // fan out per-server events.
    const fakeMetrics = {
      servers: {
        'server-abc': {
          cpuPercent: 45.2,
          memoryBytes: BigInt(1073741824),
          numberOfThreads: 12,
          diskUsageBytes: BigInt(5368709120),
          uptimeSeconds: BigInt(3600),
          memoryWorkingSetBytes: BigInt(536870912),
          memoryPercent: 25.0,
          cpuCores: 4,
          ioReadRate: 100.5,
          ioWriteRate: 50.3,
          connectionCount: 8,
        },
        'server-def': {
          cpuPercent: 10.0,
          memoryBytes: BigInt(2147483648),
          numberOfThreads: 6,
          diskUsageBytes: BigInt(10737418240),
          uptimeSeconds: BigInt(7200),
          memoryWorkingSetBytes: BigInt(1073741824),
          memoryPercent: 50.0,
          cpuCores: 8,
          ioReadRate: 200.0,
          ioWriteRate: 75.0,
          connectionCount: 3,
        },
      },
    }

    // Emit the bulk event (simulating WebSocket message arrival).
    XylonaEventBus.emit('gameServerMetrics', fakeMetrics as unknown as AllServersMetrics)

    // The handler should have fanned out two remoteServerMetrics events,
    // one per server in the map.
    expect(perServerHandler).toHaveBeenCalledTimes(2)
    expect(perServerHandler).toHaveBeenCalledWith(
      'server-abc',
      expect.objectContaining({ cpuPercent: 45.2 }),
    )
    expect(perServerHandler).toHaveBeenCalledWith(
      'server-def',
      expect.objectContaining({ cpuPercent: 10.0 }),
    )
  })

  it('should not emit remoteServerMetrics when gameServerMetrics has no servers', () => {
    const perServerHandler = vi.fn()
    XylonaEventBus.on('remoteServerMetrics', perServerHandler)

    const emptyMetrics = { servers: {} }
    XylonaEventBus.emit('gameServerMetrics', emptyMetrics as unknown as AllServersMetrics)

    expect(perServerHandler).not.toHaveBeenCalled()
  })
})

describe('dispatchWebsocketMessage', () => {
  afterEach(() => {
    XylonaEventBus.off('gameServerVersion')
    XylonaEventBus.off('gameServerBackupProgress')
  })

  it('maps game server version websocket payloads into event bus updates', () => {
    const handler = vi.fn()
    XylonaEventBus.on('gameServerVersion', handler)

    const handled = dispatchWebsocketMessage(
      create(MessageSchema, {
        type: Message_Type.GameServerVersion,
        gameServerVersionUpdate: {
          gameServerId: 'server-123',
          version: '1.21.1',
          versionInfo: {
            status: VersionStatus.CHECKED,
            installedVersion: '1.21.1',
            latestVersion: '1.21.3',
            updateAvailable: true,
            lastCheckTime: 5n,
            trackerType: 'minecraft',
          },
        },
      }),
    )

    expect(handled).toBe(true)
    expect(handler).toHaveBeenCalledWith(
      'server-123',
      '1.21.1',
      expect.objectContaining({
        installedVersion: '1.21.1',
        latestVersion: '1.21.3',
        updateAvailable: true,
      }),
    )
  })

  it('maps game server backup progress websocket payloads into event bus updates', () => {
    const handler = vi.fn()
    XylonaEventBus.on('gameServerBackupProgress', handler)

    const handled = dispatchWebsocketMessage(
      create(MessageSchema, {
        type: Message_Type.GameServerBackupProgress,
        backupProgress: {
          gameServerId: 'server-123',
          backupId: 'backup-456',
          operation: BackupProgressOperation.CREATE,
          phase: BackupProgressPhase.ARCHIVING,
          percent: 52,
          sizeBytes: 2048n,
          message: 'Archiving world data',
        },
      }),
    )

    expect(handled).toBe(true)
    expect(handler).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-123',
        backupId: 'backup-456',
        operation: BackupProgressOperation.CREATE,
        phase: BackupProgressPhase.ARCHIVING,
        percent: 52,
        sizeBytes: 2048n,
        message: 'Archiving world data',
      }),
    )
  })
})
