import { create, fromJsonString, toJsonString } from '@bufbuild/protobuf'
import type { ConnectError } from '@connectrpc/connect'
import { SystemUpdateProgress, UpdateProgress } from '@/proto/xylona_pb'
import type { BackupProgress, VersionInfo } from '@/proto/shared_pb'
import { AllServersQueryInfo, Status } from '@/proto/shared_pb'
import { GameServerFilesCompressionType } from '@/proto/gameserver_files_operations_pb'
import {
  tabArchive,
  tabBinary,
  tabBrandPowershell,
  tabBrandWindows,
  tabFileFilled,
  tabFileCode,
  tabFileDescription,
  tabFileSettings,
  tabFileTypeCss,
  tabFileTypeHtml,
  tabFileTypeJs,
  tabFileTypeJpg,
  tabFileTypePng,
  tabFileTypeSql,
  tabFileTypeTs,
  tabFileTypeTxt,
  tabFileTypeXml,
  tabFileTypeZip,
  tabFileZip,
  tabFilterSearch,
  tabIcons,
  tabJson,
  tabMarkdown,
  tabTerminal2,
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
import {
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
  websocketBrowserOnline,
  websocketConnectionStatus,
  websocketHasConnected,
} from './websocket-connection'

export const LocalXylonaWebsocketBaseURL: string = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/websocket`

const allAPIWebsockets: Map<string, ReconnectingWebSocket> = new Map<
  string,
  ReconnectingWebSocket
>()
// How long a refocused socket may stay authoritative while its probe is outstanding.
// Comfortably above a healthy round trip and well under the websocket's own pong timeout.
const LIVENESS_GRACE_MS = 1_500

let browserLifecycleInitialized = false
let pageLifecyclePaused = false
let controllerFrameCount = 0
let livenessGraceTimer: ReturnType<typeof setTimeout> | null = null

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
    apiWebsocket = new ReconnectingWebSocket(baseURL, [], {
      startPaused: pageLifecyclePaused || !navigator.onLine,
    })
    allAPIWebsockets.set(baseURL, apiWebsocket)
    setupWebsocket(apiWebsocket, baseURL === LocalXylonaWebsocketBaseURL)
    setupBrowserWebsocketLifecycle()
  }
  if (!apiWebsocket) {
    throw new Error(`WebSocket client was not initialized for ${baseURL}`)
  }
  return apiWebsocket
}

export function reconnectControllerWebsocket(): void {
  if (!websocketBrowserOnline.value) {
    return
  }

  const controllerWebsocket = GetOrCreateXylonaWebsocketClient()
  transitionControllerConnection(websocketHasConnected.value ? 'reconnecting' : 'connecting')
  controllerWebsocket.reconnectNow()
}

function setupWebsocket(apiWebsocket: ReconnectingWebSocket, isControllerSocket: boolean) {
  if (isControllerSocket) {
    if (!navigator.onLine) {
      transitionControllerConnection('disconnected')
    }
  }

  apiWebsocket.onopen = (_event) => {
    if (isControllerSocket) {
      transitionControllerConnection('connected')
    }
    console.debug('Websocket opened')
  }
  apiWebsocket.onmessage = (event) => {
    if (isControllerSocket) {
      controllerFrameCount++
      if (websocketBrowserOnline.value && websocketConnectionStatus.value !== 'connected') {
        transitionControllerConnection('connected')
      }
    }
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
      clearLivenessGrace()
      demoteControllerConnection()
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

function transitionControllerConnection(status: typeof websocketConnectionStatus.value): void {
  const previousStatus = websocketConnectionStatus.value
  if (!setWebsocketConnectionStatus(status)) {
    return
  }

  if (status === 'connected') {
    XylonaEventBus.emit('websocketConnected')
    return
  }
  if (previousStatus === 'connected') {
    XylonaEventBus.emit('websocketDisconnected')
  }
}

function setupBrowserWebsocketLifecycle(): void {
  if (browserLifecycleInitialized) {
    return
  }
  browserLifecycleInitialized = true
  setWebsocketBrowserOnline(navigator.onLine)

  window.addEventListener('offline', handleBrowserOffline)
  window.addEventListener('online', handleBrowserOnline)
  window.addEventListener('pagehide', handlePageHide)
  window.addEventListener('pageshow', handlePageShow)
  document.addEventListener('visibilitychange', handleVisibilityChange)
}

function handleBrowserOffline(): void {
  clearLivenessGrace()
  setWebsocketBrowserOnline(false)
  transitionControllerConnection('disconnected')
  for (const websocket of allAPIWebsockets.values()) {
    websocket.pause()
  }
}

function handleBrowserOnline(): void {
  setWebsocketBrowserOnline(true)
  if (pageLifecyclePaused) {
    return
  }
  restoreOrProbeWebsockets()
}

function handlePageHide(): void {
  pageLifecyclePaused = true
  clearLivenessGrace()
  demoteControllerConnection()
  for (const websocket of allAPIWebsockets.values()) {
    websocket.pause()
  }
}

function handlePageShow(): void {
  pageLifecyclePaused = false
  setWebsocketBrowserOnline(navigator.onLine)
  if (!websocketBrowserOnline.value) {
    handleBrowserOffline()
    return
  }
  restoreOrProbeWebsockets()
}

function handleVisibilityChange(): void {
  if (document.visibilityState !== 'visible' || pageLifecyclePaused) {
    return
  }

  setWebsocketBrowserOnline(navigator.onLine)
  if (!websocketBrowserOnline.value) {
    handleBrowserOffline()
    return
  }
  restoreOrProbeWebsockets()
}

function restoreOrProbeWebsockets(): void {
  for (const websocket of allAPIWebsockets.values()) {
    websocket.probe()
  }

  const controllerWebsocket = allAPIWebsockets.get(LocalXylonaWebsocketBaseURL)
  if (controllerWebsocket === undefined) {
    return
  }
  if (controllerWebsocket.isOpen()) {
    startLivenessGrace()
    return
  }
  clearLivenessGrace()
  demoteControllerConnection()
}

/**
 * A socket that survived a hidden tab may still be a zombie, so the probe sent above
 * has to answer before we believe it. Authority is withdrawn only if nothing arrives,
 * which keeps a quick tab switch from cycling every consumer through
 * disconnect/reconnect. An unanswered probe still leaves the websocket's own pong
 * timeout to perform the actual teardown.
 */
function startLivenessGrace(): void {
  clearLivenessGrace()
  const probedFrameCount = controllerFrameCount
  livenessGraceTimer = setTimeout(() => {
    livenessGraceTimer = null
    if (controllerFrameCount !== probedFrameCount) {
      return
    }
    if (pageLifecyclePaused || !websocketBrowserOnline.value) {
      return
    }
    demoteControllerConnection()
  }, LIVENESS_GRACE_MS)
}

function clearLivenessGrace(): void {
  if (livenessGraceTimer === null) {
    return
  }
  clearTimeout(livenessGraceTimer)
  livenessGraceTimer = null
}

function demoteControllerConnection(): void {
  transitionControllerConnection(
    websocketBrowserOnline.value
      ? websocketHasConnected.value
        ? 'reconnecting'
        : 'connecting'
      : 'disconnected',
  )
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

const FILE_TYPE_ICONS: Record<string, string> = {
  bat: tabTerminal2,
  bin: tabBinary,
  bz2: tabFileZip,
  cfg: tabFileSettings,
  cjs: tabFileTypeJs,
  cmd: tabTerminal2,
  conf: tabFileSettings,
  config: tabFileSettings,
  css: tabFileTypeCss,
  dll: tabBinary,
  dylib: tabBinary,
  env: tabFileSettings,
  exe: tabBrandWindows,
  gz: tabFileZip,
  html: tabFileTypeHtml,
  htm: tabFileTypeHtml,
  ico: tabIcons,
  ini: tabFileSettings,
  jar: tabArchive,
  jpeg: tabFileTypeJpg,
  js: tabFileTypeJs,
  jpg: tabFileTypeJpg,
  json: tabJson,
  license: tabFileDescription,
  log: tabFilterSearch,
  markdown: tabMarkdown,
  md: tabMarkdown,
  mjs: tabFileTypeJs,
  properties: tabFileSettings,
  png: tabFileTypePng,
  ps1: tabBrandPowershell,
  py: tabFileCode,
  readme: tabMarkdown,
  sh: tabTerminal2,
  so: tabBinary,
  sql: tabFileTypeSql,
  settings: tabFileSettings,
  toml: tabFileCode,
  ts: tabFileTypeTs,
  txt: tabFileTypeTxt,
  xml: tabFileTypeXml,
  xz: tabFileZip,
  yaml: tabFileCode,
  yml: tabFileCode,
  zip: tabFileTypeZip,
  zst: tabFileZip,
}

function getFileTypeKey(fileName: string): string {
  const normalizedName = fileName.toLocaleLowerCase()
  const extensionIndex = normalizedName.lastIndexOf('.')
  return extensionIndex < 0 ? normalizedName : normalizedName.slice(extensionIndex + 1)
}

export function getIconFromFilenameExtension(fileName: string): string {
  return FILE_TYPE_ICONS[getFileTypeKey(fileName)] ?? tabFileFilled
}

/** Maps file extensions to icon colors. */
const FILE_TYPE_COLORS: Record<string, string> = {
  bat: 'var(--xy-success)',
  bin: 'var(--xy-primary-hover)',
  bz2: 'var(--xy-purple)',
  cfg: 'var(--xy-warning)',
  cjs: 'var(--xy-warning)',
  cmd: 'var(--xy-success)',
  conf: 'var(--xy-warning)',
  config: 'var(--xy-warning)',
  css: 'var(--xy-primary-hover)',
  dll: 'var(--xy-primary-hover)',
  dylib: 'var(--xy-primary-hover)',
  env: 'var(--xy-warning)',
  exe: 'var(--xy-primary-hover)',
  gz: 'var(--xy-purple)',
  html: 'var(--xy-accent)',
  htm: 'var(--xy-accent)',
  ico: 'var(--xy-purple)',
  ini: 'var(--xy-warning)',
  jar: 'var(--xy-purple)',
  jpeg: 'var(--xy-purple)',
  js: 'var(--xy-warning)',
  jpg: 'var(--xy-purple)',
  json: 'var(--xy-success)',
  license: 'var(--xy-text-secondary)',
  log: 'var(--xy-text-muted)',
  markdown: 'var(--xy-info)',
  md: 'var(--xy-info)',
  mjs: 'var(--xy-warning)',
  properties: 'var(--xy-warning)',
  png: 'var(--xy-purple)',
  ps1: 'var(--xy-primary-hover)',
  py: 'var(--xy-accent)',
  readme: 'var(--xy-info)',
  settings: 'var(--xy-warning)',
  sh: 'var(--xy-success)',
  so: 'var(--xy-primary-hover)',
  sql: 'var(--xy-accent)',
  toml: 'var(--xy-accent)',
  ts: 'var(--xy-primary-hover)',
  txt: 'var(--xy-text-secondary)',
  xml: 'var(--xy-accent)',
  xz: 'var(--xy-purple)',
  yaml: 'var(--xy-accent)',
  yml: 'var(--xy-accent)',
  zip: 'var(--xy-purple)',
  zst: 'var(--xy-purple)',
}

const FILE_TYPE_COLOR_DEFAULT = 'var(--xy-text-primary)'

export function getColorFromFilenameExtension(fileName: string): string {
  return FILE_TYPE_COLORS[getFileTypeKey(fileName)] ?? FILE_TYPE_COLOR_DEFAULT
}
