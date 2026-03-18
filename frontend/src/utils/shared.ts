import { create, fromJsonString, toJsonString } from '@bufbuild/protobuf'
import { Code, ConnectError, createCallbackClient, createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { Xylona } from 'src/proto/xylona_pb'
import { onMounted, ref } from 'vue'
import { AllServersQueryInfo, Status } from 'src/proto/shared_pb'
import { GameServerFilesCompressionType } from 'src/proto/gameserver_files_operations_pb'
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
  Message,
  Message_Type,
  MessageSchema,
  Request,
  Request_Type,
  RequestSchema,
} from '../proto/websocket_pb'
import { EventBus } from 'quasar'
import { ReconnectingWebSocket } from './websocket'

export const LocalXylonaAPIBaseURL: string = `${window.location.protocol}//${window.location.host}`
export const LocalXylonaWebsocketBaseURL: string = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/websocket`

const allAPIWebsockets: Map<string, ReconnectingWebSocket> = new Map<
  string,
  ReconnectingWebSocket
>()

type XylonaEventBusEvents = {
  gameServerStatus: (gameServerId: string, status: Status) => void
  gameServerConsoleOutput: (gameServerId: string, consoleOutput: string) => void
  gameServerConsoleOutputRequest: (gameServerId: string) => void
  gameServersQueryInfo: (queryInfo: AllServersQueryInfo) => void
  gameServerMetrics: (metrics: AllServersMetrics) => void
  nodeMetrics: (metrics: AllNodeMetrics) => void
  websocketConnected: () => void
  websocketDisconnected: () => void
}

/**
 * Event bus for Xylona events. Used to easily communicate between components.
 * @type {EventBus<XylonaEventBusEvents>}
 */
export const XylonaEventBus: EventBus<XylonaEventBusEvents> = new EventBus<XylonaEventBusEvents>()

export function GetXylonaClient(nodeAddress: string = window.location.host) {
  const baseURL =
    nodeAddress === window.location.host
      ? LocalXylonaAPIBaseURL
      : `${window.location.protocol}//${nodeAddress}`
  const transport = createConnectTransport({
    baseUrl: baseURL,
    fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
  })
  return createClient(Xylona, transport)
}

export function GetXylonaClientCallback(nodeAddress: string = window.location.host) {
  const baseURL =
    nodeAddress === window.location.host
      ? LocalXylonaAPIBaseURL
      : `${window.location.protocol}//${nodeAddress}`
  const transport = createConnectTransport({
    baseUrl: baseURL,
    fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
  })
  return createCallbackClient(Xylona, transport)
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
    setupWebsocket(apiWebsocket)
  }
  return apiWebsocket
}

function setupWebsocket(apiWebsocket: ReconnectingWebSocket) {
  window.addEventListener('pagehide', () => {
    console.debug('Page hide event. Closing websocket...')
    apiWebsocket.close()
  })
  apiWebsocket.onopen = (event) => {
    XylonaEventBus.emit('websocketConnected')
    console.debug('Websocket opened')
  }
  apiWebsocket.onmessage = (event) => {
    if (typeof event.data === 'string' && event.data === 'pong') {
      return
    }
    const out: Message = fromJsonString(MessageSchema, event.data)
    switch (out.type) {
      case Message_Type.GameServerStatus:
        XylonaEventBus.emit(
          'gameServerStatus',
          out.gameServerStatusUpdate?.gameServerId,
          out.gameServerStatusUpdate?.status,
        )
        break
      case Message_Type.GameServerConsole:
        XylonaEventBus.emit(
          'gameServerConsoleOutput',
          out.gameServerConsoleOutput?.gameServerId,
          out.gameServerConsoleOutput?.output,
        )
        break
      case Message_Type.ServerQueries:
        XylonaEventBus.emit('gameServersQueryInfo', out.allServersQueryInfo)
        break
      case Message_Type.GameServerMetrics:
        XylonaEventBus.emit('gameServerMetrics', out.allServersMetrics)
        break
      case Message_Type.NodeMetrics:
        XylonaEventBus.emit('nodeMetrics', out.allNodeMetrics)
        break
      default:
        console.debug(`${event.data}`)
        return
    }
  }
  apiWebsocket.onclose = (event) => {
    XylonaEventBus.emit('websocketDisconnected')
    console.debug('Websocket closed')
    // Let the ReconnectingWebSocket handle the rest.
  }
  apiWebsocket.onerror = (event) => {
    console.error(event)
    // Let the ReconnectingWebSocket handle the rest.
  }

  // Handle MessageBus events
  XylonaEventBus.on('gameServerConsoleOutputRequest', (gameServerId: string) => {
    const consoleOutputRequest: Request = create(RequestSchema, {})
    consoleOutputRequest.type = Request_Type.GetGameServerConsole
    consoleOutputRequest.gameServerId = gameServerId

    apiWebsocket?.send(toJsonString(RequestSchema, consoleOutputRequest))
  })
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
    window.addEventListener('resize', () => updateWindowWidth())
    window.removeEventListener('resize', () => updateWindowWidth())
  })

  return windowWidth
}

export function StatusToString(status: Status): string {
  switch (status) {
    case Status.UNKNOWN:
      return 'Unknown'
    case Status.ONLINE:
      return 'Online'
    case Status.OFFLINE:
      return 'Offline'
    case Status.UPDATING:
      return 'Updating'
    case Status.INSTALLING:
      return 'Installing'
    default:
      return 'Unknown'
  }
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

// The conversion function
export function bytesToSize1(bytes: number): string {
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  if (bytes === 0) return '0 Bytes'
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + sizes[i]
}

export function bytesToSize(bytes: number): string {
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  if (bytes === 0) return '0 Byte'
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + sizes[i]
}

export function ConnectErrorToString(err: ConnectError): string {
  switch (err.code) {
    case Code.Unavailable:
      return 'Unable to connect to Xylona backend.'
    default:
      return err.message
  }
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

export function getColorFromFilenameExtension(fileName: string): string {
  const fileNameSplit = fileName.split('.')
  if (fileNameSplit.length <= 1) {
    return 'whitesmoke'
  }
  const extension = fileNameSplit[fileNameSplit.length - 1]
  switch (extension) {
    case 'json':
      return '#74c639'
    case 'txt':
      return '#94c2e6'
    case 'log':
      return '#818181'
    case 'settings':
      return 'orange'
    case 'jar':
      return '#f0db4f'
    case 'zip':
      return '#f0db4f'
    case 'xz':
      return '#3e9b00'
    case 'gz':
      return '#674753'
    case 'bz2':
      return '#757de7'
    case 'zst':
      return '#f07f4f'
    default:
      return 'whitesmoke'
  }
}
