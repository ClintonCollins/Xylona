import { create, fromJsonString, toJsonString } from '@bufbuild/protobuf'
import type { ConnectError } from '@connectrpc/connect'
import { SystemUpdateProgress, UpdateProgress } from '@/proto/xylona_pb'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { BackupProgress, VersionInfo } from '@/proto/shared_pb'
import { AllServersQueryInfo, Status } from '@/proto/shared_pb'
import { GameServerFilesCompressionType } from '@/proto/gameserver_files_operations_pb'
import {
  tabArchive,
  tabFileFilled,
  tabFileSettings,
  tabFileTypeTxt,
  tabFileTypeZip,
  tabFileZip,
  tabFilterSearch,
  tabJson,
} from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
  AllNodeMetrics,
  AllServersMetrics,
  GameServerMetrics,
  Message,
  Message_Type,
  MessageSchema,
  Request,
  Request_Type,
  RequestSchema,
} from '../proto/websocket_pb'
import { EventBus } from 'quasar'

import { getXylonaClient, getXylonaClientCallback } from '@/api/connect-client'
import { connectErrorToString } from '@/api/connect-errors'
import { ReconnectingWebSocket } from './websocket'
import { setWebsocketConnectionStatus, websocketHasConnected } from './websocket-connection'

export const LocalXylonaWebsocketBaseURL: string = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/websocket`

const allAPIWebsockets: Map<string, ReconnectingWebSocket> = new Map<
  string,
  ReconnectingWebSocket
>()

type XylonaEventBusEvents = {
  gameServerStatus: (gameServerId: string, gameServerName: string, status: Status) => void
  gameServerVersion: (
    gameServerId: string,
    version: string,
    versionInfo: VersionInfo | undefined,
  ) => void
  gameServerConsoleOutput: (
    gameServerId: string,
    consoleOutput: string,
    sequence: bigint,
    resetBuffer: boolean,
    reconnecting: boolean | undefined,
  ) => void
  gameServerConsoleOutputRequest: (gameServerId: string) => void
  gameServerConsoleOutputRemoveRequest: (gameServerId: string) => void
  gameServersQueryInfo: (queryInfo: AllServersQueryInfo) => void
  gameServerMetrics: (metrics: AllServersMetrics) => void
  remoteServerMetrics: (gameServerId: string, metrics: GameServerMetrics) => void
  nodeMetrics: (metrics: AllNodeMetrics) => void
  websocketConnected: () => void
  websocketDisconnected: () => void
  serverSoftwareInstall: (
    gameServerId: string,
    gameServerName: string,
    status: string,
    error: string,
    softwareId: string,
  ) => void
  gameServerUpdateProgress: (progress: UpdateProgress) => void
  gameServerBackupProgress: (progress: BackupProgress) => void
  systemUpdateProgress: (progress: SystemUpdateProgress) => void
}

/**
 * Event bus for Xylona events. Used to easily communicate between components.
 * @type {EventBus<XylonaEventBusEvents>}
 */
export const XylonaEventBus: EventBus<XylonaEventBusEvents> = new EventBus<XylonaEventBusEvents>()

// Fan out per-server metrics events so individual views can subscribe by server ID.
XylonaEventBus.on('gameServerMetrics', (metrics: AllServersMetrics) => {
  if (metrics?.servers) {
    for (const [serverId, serverMetrics] of Object.entries(metrics.servers)) {
      XylonaEventBus.emit('remoteServerMetrics', serverId, serverMetrics)
    }
  }
})

export function GetXylonaClient(nodeAddress: string = window.location.host) {
  return getXylonaClient(nodeAddress)
}

export function GetXylonaClientCallback(nodeAddress: string = window.location.host) {
  return getXylonaClientCallback(nodeAddress)
}

export function GetOrCreateXylonaWebsocketClient(
  nodeAddress: string = window.location.host,
): ReconnectingWebSocket {
  const baseURL =
    nodeAddress === window.location.host
      ? LocalXylonaWebsocketBaseURL
      : `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${nodeAddress}/api/websocket`
  let apiWebsocket: ReconnectingWebSocket | undefined = allAPIWebsockets.get(baseURL)
  const websocketInitialized = allAPIWebsockets.has(baseURL)
  if (!websocketInitialized) {
    apiWebsocket = new ReconnectingWebSocket(baseURL, [], 10000, 30000)
    allAPIWebsockets.set(baseURL, apiWebsocket)
    setupWebsocket(apiWebsocket, baseURL === LocalXylonaWebsocketBaseURL)
  }
  if (!apiWebsocket) {
    throw new Error(`WebSocket client was not initialized for ${baseURL}`)
  }
  return apiWebsocket
}

function setupWebsocket(apiWebsocket: ReconnectingWebSocket, isControllerSocket: boolean) {
  window.addEventListener('pagehide', () => {
    console.debug('Page hide event. Closing websocket...')
    apiWebsocket.close()
  })

  if (isControllerSocket) {
    if (!navigator.onLine) {
      setWebsocketConnectionStatus('disconnected')
    }
    window.addEventListener('offline', () => {
      setWebsocketConnectionStatus('disconnected')
    })
    window.addEventListener('online', () => {
      if (apiWebsocket.isOpen()) {
        setWebsocketConnectionStatus('connected')
        return
      }
      setWebsocketConnectionStatus(websocketHasConnected.value ? 'reconnecting' : 'connecting')
    })
  }

  apiWebsocket.onopen = (_event) => {
    if (isControllerSocket) {
      setWebsocketConnectionStatus('connected')
      XylonaEventBus.emit('websocketConnected')
    }
    console.debug('Websocket opened')
  }
  apiWebsocket.onmessage = (event) => {
    if (typeof event.data === 'string' && event.data === 'pong') {
      return
    }
    const out: Message = fromJsonString(MessageSchema, event.data)
    if (!dispatchWebsocketMessage(out)) {
      console.debug(`${event.data}`)
    }
  }
  apiWebsocket.onclose = (_event) => {
    if (isControllerSocket) {
      setWebsocketConnectionStatus(
        navigator.onLine
          ? websocketHasConnected.value
            ? 'reconnecting'
            : 'connecting'
          : 'disconnected',
      )
      XylonaEventBus.emit('websocketDisconnected')
    }
    console.debug('Websocket closed')
    // Let the ReconnectingWebSocket handle the rest.
  }
  apiWebsocket.onerror = (event) => {
    console.error(event)
    // Let the ReconnectingWebSocket handle the rest.
  }

  // These listeners intentionally live for the websocket instance lifetime.
  // GetOrCreateXylonaWebsocketClient() only calls setupWebsocket() once per URL.
  // Handle MessageBus events
  XylonaEventBus.on('gameServerConsoleOutputRequest', (gameServerId: string) => {
    const consoleOutputRequest: Request = create(RequestSchema, {})
    consoleOutputRequest.type = Request_Type.GetGameServerConsole
    consoleOutputRequest.gameServerId = gameServerId

    apiWebsocket?.send(toJsonString(RequestSchema, consoleOutputRequest))
  })
  XylonaEventBus.on('gameServerConsoleOutputRemoveRequest', (gameServerId: string) => {
    const consoleOutputRequest: Request = create(RequestSchema, {})
    consoleOutputRequest.type = Request_Type.RemoveGameServerConsole
    consoleOutputRequest.gameServerId = gameServerId

    apiWebsocket?.send(toJsonString(RequestSchema, consoleOutputRequest))
  })
}

export function dispatchWebsocketMessage(out: Message): boolean {
  switch (out.type) {
    case Message_Type.GameServerStatus: {
      const statusUpdate = out.gameServerStatusUpdate
      if (statusUpdate) {
        XylonaEventBus.emit(
          'gameServerStatus',
          statusUpdate.gameServerId,
          statusUpdate.gameServerName,
          statusUpdate.status,
        )
      }
      return true
    }
    case Message_Type.GameServerVersion: {
      const versionUpdate = out.gameServerVersionUpdate
      if (versionUpdate) {
        XylonaEventBus.emit(
          'gameServerVersion',
          versionUpdate.gameServerId,
          versionUpdate.version,
          versionUpdate.versionInfo,
        )
      }
      return true
    }
    case Message_Type.GameServerConsole: {
      const consoleOutput = out.gameServerConsoleOutput
      if (consoleOutput) {
        XylonaEventBus.emit(
          'gameServerConsoleOutput',
          consoleOutput.gameServerId,
          consoleOutput.output,
          consoleOutput.sequence,
          consoleOutput.resetBuffer,
          consoleOutput.reconnecting,
        )
      }
      return true
    }
    case Message_Type.ServerQueries:
      if (out.allServersQueryInfo) {
        XylonaEventBus.emit('gameServersQueryInfo', out.allServersQueryInfo)
      }
      return true
    case Message_Type.GameServerMetrics:
      if (out.allServersMetrics) {
        XylonaEventBus.emit('gameServerMetrics', out.allServersMetrics)
      }
      return true
    case Message_Type.NodeMetrics:
      if (out.allNodeMetrics) {
        XylonaEventBus.emit('nodeMetrics', out.allNodeMetrics)
      }
      return true
    case Message_Type.ServerSoftwareInstall: {
      const update = out.serverSoftwareInstallUpdate
      if (update) {
        XylonaEventBus.emit(
          'serverSoftwareInstall',
          update.gameServerId,
          update.gameServerName,
          update.status,
          update.error ?? '',
          update.softwareId ?? '',
        )
      }
      return true
    }
    case Message_Type.GameServerUpdateProgress: {
      const progress = out.updateProgress
      if (progress) {
        XylonaEventBus.emit('gameServerUpdateProgress', progress)
      }
      return true
    }
    case Message_Type.GameServerBackupProgress: {
      const progress = out.backupProgress
      if (progress) {
        XylonaEventBus.emit('gameServerBackupProgress', progress)
      }
      return true
    }
    case Message_Type.SystemUpdateProgress: {
      const progress = out.systemUpdateProgress
      if (progress) {
        XylonaEventBus.emit('systemUpdateProgress', progress)
      }
      return true
    }
    default:
      return false
  }
}

export function StringToColor(str: string): string {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash)
  }

  const hue = hash % 360
  return 'hsl(' + hue + ', 100%, 50%)'
}

export function GetRelativeFilePath(
  referencePathForSeparator: string,
  ...filePaths: string[]
): string {
  let pathSeparator = '/'
  if (referencePathForSeparator.indexOf('\\') !== -1) {
    pathSeparator = '\\'
  }
  const filteredFilePaths = filePaths.filter((path) => {
    return path !== '' && path !== undefined
  })
  if (filteredFilePaths.length < 1) {
    return ''
  }
  return filteredFilePaths.join(pathSeparator)
}

export function GetPathSeparator(path: string): string {
  if (path.indexOf('\\') !== -1) {
    return '\\'
  }
  return '/'
}

export function WindowWidth() {
  const windowWidth = ref(window.innerWidth)

  function updateWindowWidth() {
    windowWidth.value = window.innerWidth
  }

  onMounted(() => {
    window.addEventListener('resize', updateWindowWidth)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', updateWindowWidth)
  })

  return windowWidth
}

export function ArchiveTypeToString(archiveType: GameServerFilesCompressionType): string {
  switch (archiveType) {
    case GameServerFilesCompressionType.ZIP:
      return 'ZIP (.zip)'
    case GameServerFilesCompressionType.GZIP:
      return 'Gzip (.gz)'
    case GameServerFilesCompressionType.BZIP2:
      return 'Bzip2 (.bz2)'
    case GameServerFilesCompressionType.ZST:
      return 'Zstandard (.zst)'
    case GameServerFilesCompressionType.XZ:
      return 'XZ (.xz)'
    default:
      return 'Unknown'
  }
}

export function ArchiveTypeToExtension(archiveType: GameServerFilesCompressionType): string {
  switch (archiveType) {
    case GameServerFilesCompressionType.ZIP:
      return '.zip'
    case GameServerFilesCompressionType.GZIP:
      return '.tar.gz'
    case GameServerFilesCompressionType.BZIP2:
      return '.tar.bz2'
    case GameServerFilesCompressionType.ZST:
      return '.tar.zst'
    case GameServerFilesCompressionType.XZ:
      return '.tar.xz'
    default:
      return '.unknown'
  }
}

export function bytesToSize(bytes: number): string {
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  if (bytes === 0) return '0 Byte'
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + sizes[i]
}

export function ConnectErrorToString(err: ConnectError): string {
  return connectErrorToString(err)
}

export function getIconFromFilenameExtension(fileName: string): string {
  const fileNameSplit = fileName.split('.')
  if (fileNameSplit.length <= 1) {
    return tabFileFilled
  }
  const extension = fileNameSplit[fileNameSplit.length - 1]
  switch (extension) {
    case 'json':
      return tabJson
    case 'txt':
      return tabFileTypeTxt
    case 'log':
      return tabFilterSearch
    case 'settings':
      return tabFileSettings
    case 'jar':
      return tabArchive
    case 'zip':
      return tabFileTypeZip
    case 'xz':
      return tabFileZip
    case 'gz':
      return tabFileZip
    case 'bz2':
      return tabFileZip
    case 'zst':
      return tabFileZip
    default:
      return tabFileFilled
  }
}

/** Maps file extensions to icon colors. */
const FILE_TYPE_COLORS: Record<string, string> = {
  json: '#74c639',
  txt: '#94c2e6',
  log: '#818181',
  settings: '#f59e0b',
  jar: '#f0db4f',
  zip: '#f0db4f',
  xz: '#3e9b00',
  gz: '#674753',
  bz2: '#757de7',
  zst: '#f07f4f',
}

const FILE_TYPE_COLOR_DEFAULT = '#f5f5f5'

export function getColorFromFilenameExtension(fileName: string): string {
  const fileNameSplit = fileName.split('.')
  if (fileNameSplit.length <= 1) {
    return FILE_TYPE_COLOR_DEFAULT
  }
  const extension = fileNameSplit[fileNameSplit.length - 1] ?? ''
  return FILE_TYPE_COLORS[extension] ?? FILE_TYPE_COLOR_DEFAULT
}
