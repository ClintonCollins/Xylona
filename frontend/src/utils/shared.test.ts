import { create } from '@bufbuild/protobuf'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Status, VersionStatus } from '@/proto/shared_pb'
import { GameServerFilesCompressionType } from '@/proto/gameserver_files_operations_pb'
import { AllServersMetrics, Message_Type, MessageSchema } from '@/proto/websocket_pb'

import {
  ArchiveTypeToExtension,
  ArchiveTypeToString,
  bytesToSize,
  bytesToSize1,
  dispatchWebsocketMessage,
  getColorFromFilenameExtension,
  GetPathSeparator,
  GetRelativeFilePath,
  getIconFromFilenameExtension,
  StatusToString,
  XylonaEventBus,
} from './shared'

describe('GetRelativeFilePath', () => {
  it('joins Unix paths with /', () => {
    const result = GetRelativeFilePath('/home/server', 'saves', 'world')
    expect(result).toBe('saves/world')
  })

  it('joins Windows paths with backslash', () => {
    const result = GetRelativeFilePath('C:\\server\\data', 'saves', 'world')
    expect(result).toBe('saves\\world')
  })

  it('filters empty segments', () => {
    const result = GetRelativeFilePath('/home/server', '', 'saves', '', 'world')
    expect(result).toBe('saves/world')
  })

  it('returns empty string when all segments are empty', () => {
    const result = GetRelativeFilePath('/home/server', '', '', '')
    expect(result).toBe('')
  })

  it('returns single segment unchanged', () => {
    const result = GetRelativeFilePath('/home/server', 'saves')
    expect(result).toBe('saves')
  })

  it('filters undefined segments', () => {
    const result = GetRelativeFilePath('/home/server', undefined as unknown as string, 'saves')
    expect(result).toBe('saves')
  })
})

describe('GetPathSeparator', () => {
  it('returns backslash for Windows paths', () => {
    expect(GetPathSeparator('C:\\Users\\test')).toBe('\\')
  })

  it('returns forward slash for Unix paths', () => {
    expect(GetPathSeparator('/home/user/test')).toBe('/')
  })

  it('returns forward slash when no separator found', () => {
    expect(GetPathSeparator('filename.txt')).toBe('/')
  })
})

describe('StatusToString', () => {
  it('maps UNKNOWN to "Unknown"', () => {
    expect(StatusToString(Status.UNKNOWN)).toBe('Unknown')
  })

  it('maps ONLINE to "Online"', () => {
    expect(StatusToString(Status.ONLINE)).toBe('Online')
  })

  it('maps OFFLINE to "Offline"', () => {
    expect(StatusToString(Status.OFFLINE)).toBe('Offline')
  })

  it('maps UPDATING to "Updating"', () => {
    expect(StatusToString(Status.UPDATING)).toBe('Updating')
  })

  it('maps INSTALLING to "Installing"', () => {
    expect(StatusToString(Status.INSTALLING)).toBe('Installing')
  })

  it('defaults to "Unknown" for unrecognized values', () => {
    expect(StatusToString(999 as Status)).toBe('Unknown')
  })
})

describe('bytesToSize', () => {
  it('returns "0 Byte" for 0 bytes', () => {
    expect(bytesToSize(0)).toBe('0 Byte')
  })

  it('formats bytes correctly', () => {
    expect(bytesToSize(500)).toBe('500 Bytes')
  })

  it('formats kilobytes correctly', () => {
    expect(bytesToSize(1024)).toBe('1 KB')
  })

  it('formats megabytes correctly', () => {
    expect(bytesToSize(1048576)).toBe('1 MB')
  })

  it('formats gigabytes correctly', () => {
    expect(bytesToSize(1073741824)).toBe('1 GB')
  })
})

describe('bytesToSize1', () => {
  it('returns "0 Bytes" for 0 bytes', () => {
    expect(bytesToSize1(0)).toBe('0 Bytes')
  })

  it('formats bytes correctly', () => {
    expect(bytesToSize1(500)).toBe('500.00 Bytes')
  })

  it('formats kilobytes correctly', () => {
    expect(bytesToSize1(1024)).toBe('1.00 KB')
  })

  it('formats megabytes correctly', () => {
    expect(bytesToSize1(1048576)).toBe('1.00 MB')
  })
})

describe('getIconFromFilenameExtension', () => {
  it('returns json icon for .json files', () => {
    const icon = getIconFromFilenameExtension('config.json')
    expect(icon).toBeTruthy()
  })

  it('returns txt icon for .txt files', () => {
    const icon = getIconFromFilenameExtension('readme.txt')
    expect(icon).toBeTruthy()
  })

  it('returns zip icon for .zip files', () => {
    const icon = getIconFromFilenameExtension('archive.zip')
    expect(icon).toBeTruthy()
  })

  it('returns default icon for unknown extensions', () => {
    const icon = getIconFromFilenameExtension('binary.dat')
    expect(icon).toBeTruthy()
  })

  it('returns default icon for files without extension', () => {
    const icon = getIconFromFilenameExtension('Makefile')
    expect(icon).toBeTruthy()
  })

  it('returns same icon for unknown extension and no extension', () => {
    const noExt = getIconFromFilenameExtension('Makefile')
    const unknownExt = getIconFromFilenameExtension('file.xyz')
    expect(noExt).toBe(unknownExt)
  })
})

describe('getColorFromFilenameExtension', () => {
  it('returns green for .json files', () => {
    expect(getColorFromFilenameExtension('config.json')).toBe('#74c639')
  })

  it('returns blue for .txt files', () => {
    expect(getColorFromFilenameExtension('readme.txt')).toBe('#94c2e6')
  })

  it('returns grey for .log files', () => {
    expect(getColorFromFilenameExtension('server.log')).toBe('#818181')
  })

  it('returns amber for .settings files', () => {
    expect(getColorFromFilenameExtension('user.settings')).toBe('#f59e0b')
  })

  it('returns default color for unknown extensions', () => {
    expect(getColorFromFilenameExtension('binary.dat')).toBe('#f5f5f5')
  })

  it('returns default color for files without extension', () => {
    expect(getColorFromFilenameExtension('Makefile')).toBe('#f5f5f5')
  })
})

describe('ArchiveTypeToString', () => {
  it('maps ZIP', () => {
    expect(ArchiveTypeToString(GameServerFilesCompressionType.ZIP)).toBe('ZIP (.zip)')
  })

  it('maps GZIP', () => {
    expect(ArchiveTypeToString(GameServerFilesCompressionType.GZIP)).toBe('Gzip (.gz)')
  })

  it('maps BZIP2', () => {
    expect(ArchiveTypeToString(GameServerFilesCompressionType.BZIP2)).toBe('Bzip2 (.bz2)')
  })

  it('maps ZST', () => {
    expect(ArchiveTypeToString(GameServerFilesCompressionType.ZST)).toBe('Zstandard (.zst)')
  })

  it('maps XZ', () => {
    expect(ArchiveTypeToString(GameServerFilesCompressionType.XZ)).toBe('XZ (.xz)')
  })

  it('defaults to "Unknown" for unrecognized values', () => {
    expect(ArchiveTypeToString(999 as GameServerFilesCompressionType)).toBe('Unknown')
  })
})

describe('ArchiveTypeToExtension', () => {
  it('maps ZIP to .zip', () => {
    expect(ArchiveTypeToExtension(GameServerFilesCompressionType.ZIP)).toBe('.zip')
  })

  it('maps GZIP to .tar.gz', () => {
    expect(ArchiveTypeToExtension(GameServerFilesCompressionType.GZIP)).toBe('.tar.gz')
  })

  it('maps BZIP2 to .tar.bz2', () => {
    expect(ArchiveTypeToExtension(GameServerFilesCompressionType.BZIP2)).toBe('.tar.bz2')
  })

  it('maps ZST to .tar.zst', () => {
    expect(ArchiveTypeToExtension(GameServerFilesCompressionType.ZST)).toBe('.tar.zst')
  })

  it('maps XZ to .tar.xz', () => {
    expect(ArchiveTypeToExtension(GameServerFilesCompressionType.XZ)).toBe('.tar.xz')
  })

  it('defaults to .unknown for unrecognized values', () => {
    expect(ArchiveTypeToExtension(999 as GameServerFilesCompressionType)).toBe('.unknown')
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
})
