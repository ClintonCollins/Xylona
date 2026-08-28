import { create } from '@bufbuild/protobuf'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BackupProgressOperation, BackupProgressPhase, VersionStatus } from '@/proto/shared_pb'
import { GameServerFilesCompressionType } from '@/proto/gameserver_files_operations_pb'
import { AllServersMetrics, Message_Type, MessageSchema } from '@/proto/websocket_pb'
import {
  tabBinary,
  tabBrandPowershell,
  tabBrandWindows,
  tabFileSettings,
  tabFileTypeJpg,
  tabFileTypePng,
  tabIcons,
  tabMarkdown,
  tabTerminal2,
} from 'quasar-extras-svg-icons/tabler-icons-v2'

import {
  ArchiveTypeToExtension,
  ArchiveTypeToString,
  bytesToSize,
  DisposeXylonaWebsocketClients,
  dispatchWebsocketMessage,
  getColorFromFilenameExtension,
  getIconFromFilenameExtension,
  GetOrCreateXylonaWebsocketClient,
  GetPathSeparator,
  GetRelativeFilePath,
  XylonaEventBus,
} from './shared'
import {
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
  websocketBrowserOnline,
  websocketConnectionEpoch,
  websocketConnectionStatus,
} from './websocket-connection'

class FakeLifecycleWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static instances: FakeLifecycleWebSocket[] = []

  readonly sent: string[] = []
  readyState = FakeLifecycleWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor() {
    FakeLifecycleWebSocket.instances.push(this)
  }

  send(data: string): void {
    this.sent.push(data)
  }

  close(code?: number, reason?: string): void {
    this.readyState = FakeLifecycleWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close', { code, reason }))
  }

  triggerOpen(): void {
    this.readyState = FakeLifecycleWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  triggerMessage(data: unknown): void {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  triggerClose(code: number, reason: string): void {
    this.readyState = FakeLifecycleWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close', { code, reason }))
  }
}

function getLifecycleSocket(index: number): FakeLifecycleWebSocket {
  const socket = FakeLifecycleWebSocket.instances[index]
  if (socket === undefined) {
    throw new Error(`expected lifecycle websocket ${index} to exist`)
  }
  return socket
}

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
    ['zero bytes', 0, undefined, '0 Byte'],
    ['bytes', 500, undefined, '500 Bytes'],
    ['kilobytes', 1024, undefined, '1 KB'],
    ['megabytes', 1048576, undefined, '1 MB'],
    ['gigabytes', 1073741824, undefined, '1 GB'],
    ['fixed zero', 0, 2, '0.00 Byte'],
    ['fixed trailing zeroes', 1_153_434, 2, '1.10 MB'],
  ])('formats $name', (_name, bytes, fractionDigits, want) => {
    expect(bytesToSize(bytes, fractionDigits)).toBe(want)
  })
})

describe('file type presentation', () => {
  it.each([
    ['server.EXE', tabBrandWindows, 'var(--xy-primary-hover)'],
    ['library.dll', tabBinary, 'var(--xy-primary-hover)'],
    ['server.ini', tabFileSettings, 'var(--xy-warning)'],
    ['server.cfg', tabFileSettings, 'var(--xy-warning)'],
    ['README.md', tabMarkdown, 'var(--xy-info)'],
    ['start.sh', tabTerminal2, 'var(--xy-success)'],
    ['start.bat', tabTerminal2, 'var(--xy-success)'],
    ['setup.ps1', tabBrandPowershell, 'var(--xy-primary-hover)'],
    ['preview.png', tabFileTypePng, 'var(--xy-purple)'],
    ['screenshot.jpg', tabFileTypeJpg, 'var(--xy-purple)'],
    ['photo.JPEG', tabFileTypeJpg, 'var(--xy-purple)'],
    ['favicon.ico', tabIcons, 'var(--xy-purple)'],
  ])('maps %s to a specific icon and token color', (fileName, icon, color) => {
    expect(getIconFromFilenameExtension(fileName)).toBe(icon)
    expect(getColorFromFilenameExtension(fileName)).toBe(color)
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

describe('controller websocket browser lifecycle', () => {
  it('pauses offline and for BFCache, and only withdraws authority when a probe goes unanswered', () => {
    const originalWebSocket = globalThis.WebSocket
    const originalOnline = Object.getOwnPropertyDescriptor(navigator, 'onLine')
    const originalVisibility = Object.getOwnPropertyDescriptor(document, 'visibilityState')
    const connected = vi.fn()
    const disconnected = vi.fn()

    vi.useFakeTimers()
    globalThis.WebSocket = FakeLifecycleWebSocket as unknown as typeof WebSocket
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: true })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    })
    setWebsocketBrowserOnline(true)
    setWebsocketConnectionStatus('connecting')
    XylonaEventBus.on('websocketConnected', connected)
    XylonaEventBus.on('websocketDisconnected', disconnected)

    try {
      GetOrCreateXylonaWebsocketClient()
      const initialEpoch = websocketConnectionEpoch.value
      getLifecycleSocket(0).triggerOpen()
      expect(websocketConnectionStatus.value).toBe('connected')
      expect(websocketConnectionEpoch.value).toBe(initialEpoch + 1)
      expect(connected).toHaveBeenCalledOnce()

      window.dispatchEvent(new Event('offline'))
      window.dispatchEvent(new Event('offline'))
      expect(websocketBrowserOnline.value).toBe(false)
      expect(websocketConnectionStatus.value).toBe('disconnected')
      expect(disconnected).toHaveBeenCalledOnce()
      vi.advanceTimersByTime(60_000)
      expect(FakeLifecycleWebSocket.instances).toHaveLength(1)

      window.dispatchEvent(new Event('online'))
      expect(websocketBrowserOnline.value).toBe(true)
      expect(websocketConnectionStatus.value).toBe('reconnecting')
      expect(FakeLifecycleWebSocket.instances).toHaveLength(2)
      getLifecycleSocket(1).triggerOpen()
      expect(websocketConnectionStatus.value).toBe('connected')
      expect(websocketConnectionEpoch.value).toBe(initialEpoch + 2)

      window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }))
      expect(websocketConnectionStatus.value).toBe('reconnecting')
      expect(disconnected).toHaveBeenCalledTimes(2)
      window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }))
      expect(websocketConnectionStatus.value).toBe('reconnecting')
      expect(FakeLifecycleWebSocket.instances).toHaveLength(3)
      getLifecycleSocket(2).triggerOpen()
      expect(websocketConnectionStatus.value).toBe('connected')

      Object.defineProperty(document, 'visibilityState', {
        configurable: true,
        value: 'hidden',
      })
      document.dispatchEvent(new Event('visibilitychange'))
      expect(getLifecycleSocket(2).sent).toEqual([])

      Object.defineProperty(document, 'visibilityState', {
        configurable: true,
        value: 'visible',
      })
      // Refocus probes the surviving socket but keeps authority during the grace window.
      document.dispatchEvent(new Event('visibilitychange'))
      expect(getLifecycleSocket(2).sent).toEqual(['ping'])
      expect(websocketConnectionStatus.value).toBe('connected')
      expect(disconnected).toHaveBeenCalledTimes(2)

      // A frame inside the window proves liveness, so the tab switch costs nothing.
      getLifecycleSocket(2).triggerMessage('pong')
      vi.advanceTimersByTime(5_000)
      expect(websocketConnectionStatus.value).toBe('connected')
      expect(websocketConnectionEpoch.value).toBe(initialEpoch + 3)
      expect(disconnected).toHaveBeenCalledTimes(2)

      // A socket that answers nothing loses authority once the window expires.
      document.dispatchEvent(new Event('visibilitychange'))
      expect(getLifecycleSocket(2).sent).toEqual(['ping', 'ping'])
      expect(websocketConnectionStatus.value).toBe('connected')
      vi.advanceTimersByTime(1_500)
      expect(websocketConnectionStatus.value).toBe('reconnecting')
      expect(disconnected).toHaveBeenCalledTimes(3)

      XylonaEventBus.off('websocketConnected', connected)
      XylonaEventBus.off('websocketDisconnected', disconnected)
      DisposeXylonaWebsocketClients()
    } finally {
      XylonaEventBus.off('websocketConnected', connected)
      XylonaEventBus.off('websocketDisconnected', disconnected)
      setWebsocketConnectionStatus('connecting')
      setWebsocketBrowserOnline(true)
      globalThis.WebSocket = originalWebSocket
      FakeLifecycleWebSocket.instances = []
      vi.clearAllTimers()
      vi.useRealTimers()
      if (originalOnline === undefined) {
        Reflect.deleteProperty(navigator, 'onLine')
      } else {
        Object.defineProperty(navigator, 'onLine', originalOnline)
      }
      if (originalVisibility === undefined) {
        Reflect.deleteProperty(document, 'visibilityState')
      } else {
        Object.defineProperty(document, 'visibilityState', originalVisibility)
      }
    }
  })

  it('disposes and evicts cached sockets when authentication changes', () => {
    const originalWebSocket = globalThis.WebSocket

    globalThis.WebSocket = FakeLifecycleWebSocket as unknown as typeof WebSocket
    setWebsocketBrowserOnline(true)
    setWebsocketConnectionStatus('connecting')

    try {
      const controllerClient = GetOrCreateXylonaWebsocketClient()
      GetOrCreateXylonaWebsocketClient('node.example.test')
      expect(FakeLifecycleWebSocket.instances).toHaveLength(2)

      DisposeXylonaWebsocketClients()

      expect(FakeLifecycleWebSocket.instances.every((socket) => socket.readyState === 3)).toBe(true)
      expect(websocketConnectionStatus.value).toBe('disconnected')

      const replacementClient = GetOrCreateXylonaWebsocketClient()
      expect(replacementClient).not.toBe(controllerClient)
      expect(FakeLifecycleWebSocket.instances).toHaveLength(3)
      getLifecycleSocket(2).triggerOpen()
      expect(() =>
        XylonaEventBus.emit('gameServerConsoleOutputRequest', 'server-123'),
      ).not.toThrow()
      expect(getLifecycleSocket(2).sent).toHaveLength(1)
    } finally {
      DisposeXylonaWebsocketClients()
      setWebsocketConnectionStatus('connecting')
      setWebsocketBrowserOnline(true)
      globalThis.WebSocket = originalWebSocket
      FakeLifecycleWebSocket.instances = []
    }
  })

  it('stops reconnecting and redirects when the session expires', () => {
    const originalWebSocket = globalThis.WebSocket
    const assign = vi.spyOn(window.location, 'assign').mockImplementation(() => undefined)

    vi.useFakeTimers()
    globalThis.WebSocket = FakeLifecycleWebSocket as unknown as typeof WebSocket
    window.history.replaceState({}, '', '/dashboard')
    setWebsocketBrowserOnline(true)
    setWebsocketConnectionStatus('connecting')

    try {
      GetOrCreateXylonaWebsocketClient()
      getLifecycleSocket(0).triggerOpen()
      getLifecycleSocket(0).triggerClose(4003, 'Session expired')

      expect(assign).toHaveBeenCalledOnce()
      expect(assign).toHaveBeenCalledWith('/login?reason=session-expired')
      expect(websocketConnectionStatus.value).toBe('disconnected')
      vi.advanceTimersByTime(60_000)
      expect(FakeLifecycleWebSocket.instances).toHaveLength(1)
    } finally {
      DisposeXylonaWebsocketClients()
      setWebsocketConnectionStatus('connecting')
      setWebsocketBrowserOnline(true)
      globalThis.WebSocket = originalWebSocket
      FakeLifecycleWebSocket.instances = []
      assign.mockRestore()
      vi.clearAllTimers()
      vi.useRealTimers()
    }
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
