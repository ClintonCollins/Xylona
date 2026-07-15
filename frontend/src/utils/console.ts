import { StringToColor } from '@/utils/shared'

// These regexes intentionally live at module scope for reuse. Any future
// exec()/test() call against a stateful regex must reset lastIndex first.
const reURIMatch =
  /(?:(?:https?|ftp|file):\/\/|www\.|ftp\.)(?:\([-A-Z0-9+&@#/%=~_|$?!:,.]*\)|[-A-Z0-9+&@#/%=~_|$?!:,.])*(?:\([-A-Z0-9+&@#/%=~_|$?!:,.]*\)|[A-Z0-9+&@#/%=~_|$])/gim
const reServerStop = /Server stopped.+$/gim
const reExitStatus = /^exit status [1-9]|^exit status 0.+$/gim
const reInfo = /^INFO|INF/gm
const reWarn = /^WARNING|WARN|WRN/gm
const reError = /ERROR/gm
const reXylonaMessage = /\[(\d+-\d+-\d+\s\d+:\d+:\d+)]\s\[(Xylona)]/gm
const reANSIEscape = new RegExp(`${String.fromCodePoint(27)}\\[[0-?]*[ -/]*[@-~]`, 'g')

// SteamCMD console regex
const reSteamCMDLauncher =
  /^(\s*)(steamcmd\.sh\[\d+]:)(\s+)(Starting|Restarting steamcmd by request)(.*)$/i
const reSteamCMDPath = /^(\s*)(Redirecting stderr to|Logging directory:)(\s+)(.+)$/i
const reSteamCMDBootstrapProgress = /^(\s*)(\[\s*(?:\d+%|----)])(\s+)(.+)$/
const reSteamCMDDownloadProgress = /^(.*?)(\s+\()([\d,]+)(\s+of\s+)([\d,]+\s+KB)(\))(\.\.\.)?$/i
const reSteamCMDUpdateState =
  /^(\s*)(Update state)(\s+\(0x[\da-f]+\))(\s+)([^,]+)(,\s+progress:\s+)([\d.]+)(\s+\()([\d,]+)(\s+\/\s+)([\d,]+)(\))$/i
const reSteamCMDConnection =
  /^(\s*)(Loading Steam API|Connecting anonymously to Steam Public|Waiting for client config|Waiting for user info)(\.\.\.)(OK)?$/i
const reSteamCMDClient = /^(\s*)(Steam Console Client)(.*?)(version\s+)(\d+)(.*)$/i
const reSteamCMDHint = /^(\s*--\s+type\s+)('quit')(\s+to exit\s+--\s*)$/i
const reSteamCMDUpdateUI = /^(\s*)(UpdateUI:)(.*)$/i
const reSteamCMDIPCWarning = /^(\s*)(IPC function call\s+)(.+?)(\s+took too long:.*)$/i
const reSteamCMDSuccess = /^(\s*)(Success!)(\s+App\s+')([\d]+)('.*)$/i
const reSteamCMDPrompt = /^(\s*)(Steam&gt;)(.*)$/i

// Palworld (Unreal Engine) console regex
const rePalworldStructuredLog =
  /^(\s*)(\[\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}])(\s+)(\[(?:LOG|INFO|WARN(?:ING)?|ERROR|FATAL|DEBUG|TRACE)])(\s+)(.*)$/i
const rePalworldDetailedLog =
  /^(\s*)(?:(\[\d{4}\.\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}:\d{3}])(\[\s*\d+]))?(\s*)(Log[A-Za-z\d_]+)(:\s*)(Fatal|Error|Warning|Display|Log|Verbose|VeryVerbose)(:\s*)(.*)$/i
const rePalworldCategoryLog =
  /^(\s*)(?:(\[\d{4}\.\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}:\d{3}])(\[\s*\d+]))?(\s*)(Log[A-Za-z\d_]+)(:\s*)(.*)$/i
const rePalworldSteamAPI = /^(\s*)(\[S_API(?: FAIL)?])(\s*)(.*)$/i
const rePalworldAppID = /^(\s*)(Setting breakpad minidump AppID)(\s*=\s*)(\d+)(.*)$/i
const rePalworldShutdownHandler = /^(\s*)(Shutdown handler:)(.*)$/i
const rePalworldStartupInfo =
  /^(\s*)(Increasing per-process limit.*|Existing per-process limit.*|Disabling core dumps\..*)$/i
const rePalworldStartupWarning =
  /^(\s*)(dlopen failed trying to load:|with error:|.*steamclient\.so: cannot open shared object file:.*)$/i
const rePalworldSteamClientOK = /^(\s*)(.*steamclient\.so.*\s)(OK)(.*)$/i
const rePalworldRESTAccess = /^(REST accessed endpoint)(\s+)(\/\S+)(\s+)(OK)(.*)$/i
const rePalworldPlayerConnection =
  /^(.+?)(\s+)(joined|left)(\s+the server\.\s+\(User id:\s+)([^,]+)(,\s+Player id:\s+)([^)]+)(\).*)$/i
const rePalworldRESTStarted = /^(REST API)(\s+)(started)(\s+on port\s+)(\d+)(.*)$/i
const rePalworldRESTStopped = /^(REST API)(\s+)(stopped)(.*)$/i
const rePalworldServerStarted = /^(Running Palworld dedicated server on)(\s+)(:\d+)(.*)$/i
const rePalworldGameVersion = /^(Game version is)(\s+)(\S+)(.*)$/i
const rePalworldRequestExit = /^(FUnixPlatformMisc::RequestExit(?:WithStatus)?)(.*)$/i
const rePalworldAbnormalExit = /^(Exiting abnormally)(\s+\(error code:\s+)(\d+)(\).*)$/i
const rePalworldExistingFile = /^(The file already exists:)(\s+)(.+)$/i
const rePalworldEngineVersion = /^(\d+\.\d+\.\d+-\d+\+{3}UE5\+Release-\d+\.\d+)(.*)$/i
const rePalworldTrailingOK = /^(.*\s)(OK)([.!]?)$/i

// V-Rising console regex
const reVRisingServer = /^\[Server]/gm
const reVRisingCompress = /^\[CompressModificationIdsOnLoadSystem]/gm
const reVRisingPersistence = /^PersistenceV2/gm
const reVRisingFinishedSaving = /(?<=Finished Saving to\s').+(?='.)/gm

// 7 Days to Die console regex
const re7dTimestamp = /^(\d+-\d+-\d+)T(\d+:\d+:\d+)( \d+.\d+)\s(INF)/gim
const re7dInfoLine = /^(.+\s)(INF\s)/gim
const re7dOS = /^(.+)(OS):(.+)$/gim
const re7dCPU = /^(.+)(CPU):(.+)$/gim
const re7dRAM = /^(.+)(RAM):(.+)$/gim
const re7dGPU = /^(.+)(GPU):(.+)$/gim
const re7dSystemInfo = /^(.+)(System information)/gim
const re7dVersion = /(Version)(:\s)(.+)$/gim
const re7dHelpCommands = /^\s(.+)\s=&gt;/gim

// Minecraft console regex
const reMcServerInfo = /(^.+)(\[.+INFO])/gm
const reMcServerWarn = /(^.+)(\[.+WARN])/gm
const reMcServerError = /(^.+)(\[.+ERROR])/gm
const reMcTimestamp = /\[\d+:\d+:\d+]/gim
const reMcPlayerJoin = /^\[.+]\s\[User Authenticator.+]:\sUUID of player\s(.+)\sis\s(.+)/gim
const reMcPlayerLeave = /^\[.+]\s\[.+]:\s(.+)\sleft the game/gim

type MinecraftPlayer = {
  username: string
  color: string
  uid: string
}

const minecraftPlayerMap: Map<string, MinecraftPlayer> = new Map<string, MinecraftPlayer>()

function execRegex(regex: RegExp, data: string): RegExpExecArray | null {
  regex.lastIndex = 0
  return regex.exec(data)
}

export function parseConsole(game: string, data: string): string {
  data = data.replaceAll(reANSIEscape, '')
  data = data.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  data = parseSteamCMDConsole(data)
  data = data.replace(reServerStop, "<span class='text-red-5'>$&</span>")
  data = data.replace(reExitStatus, "<span class='text-red-5'>$&</span>")
  switch (game.toLowerCase()) {
    case 'minecraft':
      data = parseMinecraftConsole(data)
      break
    case 'palworld':
      data = parsePalworldConsole(data)
      break
    case '7_days_to_die':
      data = parse7DaysToDieConsole(data)
      break
    case 'v-rising':
      data = parseVRisingConsole(data)
      break
    default:
      data = parseDefaultConsole(data)
      break
  }
  data = data.replaceAll(reError, "<span class='text-red-5'>$&</span>")
  data = data.replaceAll(reWarn, "<span class='text-yellow-5'>$&</span>")
  data = data.replaceAll(
    reXylonaMessage,
    "<span class='text-grey-6'>[$1]</span> <span class='text-cyan-7'>[$2]</span>",
  )
  return data
}

function consoleSpan(className: string, content: string): string {
  return `<span class='${className}'>${content}</span>`
}

function parseSteamCMDConsole(data: string): string {
  return data.replace(/[^\r\n]+/g, (line) => parseSteamCMDLine(line))
}

function parseSteamCMDLine(line: string): string {
  const launcher = line.match(reSteamCMDLauncher)
  if (launcher !== null) {
    const [, leading = '', process = '', spacing = '', action = '', remainder = ''] = launcher
    return `${leading}${consoleSpan('text-cyan-5', process)}${spacing}${consoleSpan('text-blue-4', action)}${highlightSteamCMDPath(remainder)}`
  }

  const path = line.match(reSteamCMDPath)
  if (path !== null) {
    const [, leading = '', label = '', spacing = '', value = ''] = path
    return `${leading}${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-purple-3', value)}`
  }

  const progress = line.match(reSteamCMDBootstrapProgress)
  if (progress !== null) {
    const [, leading = '', marker = '', spacing = '', status = ''] = progress
    return `${leading}${consoleSpan('text-cyan-5', marker)}${spacing}${highlightSteamCMDStatus(status)}`
  }

  const updateState = line.match(reSteamCMDUpdateState)
  if (updateState !== null) {
    const [
      ,
      leading = '',
      label = '',
      stateCode = '',
      spacing = '',
      state = '',
      progressLabel = '',
      percent = '',
      byteOpen = '',
      downloadedBytes = '',
      byteSeparator = '',
      totalBytes = '',
      byteClose = '',
    ] = updateState

    return `${leading}${consoleSpan('text-cyan-5', label)}${consoleSpan('text-purple-3', stateCode)}${spacing}${consoleSpan('text-blue-4', state)}${progressLabel}${consoleSpan('text-amber-4', percent)}${byteOpen}${consoleSpan('text-purple-3', downloadedBytes)}${byteSeparator}${consoleSpan('text-purple-3', totalBytes)}${byteClose}`
  }

  const connection = line.match(reSteamCMDConnection)
  if (connection !== null) {
    const [, leading = '', status = '', ellipsis = '', result = ''] = connection
    const highlightedResult = result.length === 0 ? '' : consoleSpan('text-green-5', result)
    return `${leading}${consoleSpan('text-cyan-5', status)}${ellipsis}${highlightedResult}`
  }

  const client = line.match(reSteamCMDClient)
  if (client !== null) {
    const [, leading = '', name = '', details = '', versionLabel = '', version = '', rest = ''] =
      client
    return `${leading}${consoleSpan('text-cyan-5', name)}${details}${versionLabel}${consoleSpan('text-purple-3', version)}${rest}`
  }

  const hint = line.match(reSteamCMDHint)
  if (hint !== null) {
    const [, leading = '', command = '', rest = ''] = hint
    return `${leading}${consoleSpan('text-amber-4', command)}${rest}`
  }

  const updateUI = line.match(reSteamCMDUpdateUI)
  if (updateUI !== null) {
    const [, leading = '', label = '', message = ''] = updateUI
    return `${leading}${consoleSpan('text-purple-3', label)}${message}`
  }

  const ipcWarning = line.match(reSteamCMDIPCWarning)
  if (ipcWarning !== null) {
    const [, leading = '', label = '', operation = '', message = ''] = ipcWarning
    return `${leading}${consoleSpan('text-yellow-5', label)}${consoleSpan('text-purple-3', operation)}${consoleSpan('text-yellow-5', message)}`
  }

  const success = line.match(reSteamCMDSuccess)
  if (success !== null) {
    const [, leading = '', label = '', appLabel = '', appID = '', rest = ''] = success
    return `${leading}${consoleSpan('text-green-5', label)}${appLabel}${consoleSpan('text-purple-3', appID)}${rest}`
  }

  const prompt = line.match(reSteamCMDPrompt)
  if (prompt !== null) {
    const [, leading = '', promptText = '', command = ''] = prompt
    return `${leading}${consoleSpan('text-cyan-5', promptText)}${consoleSpan('text-amber-4', command)}`
  }

  return line
}

function highlightSteamCMDPath(path: string): string {
  if (path.trim().length === 0) return path

  const spacing = path.match(/^\s*/)?.[0] ?? ''
  return `${spacing}${consoleSpan('text-purple-3', path.slice(spacing.length))}`
}

function highlightSteamCMDStatus(status: string): string {
  const downloadProgress = status.match(reSteamCMDDownloadProgress)
  if (downloadProgress !== null) {
    const [
      ,
      action = '',
      open = '',
      downloaded = '',
      separator = '',
      total = '',
      close = '',
      ellipsis = '',
    ] = downloadProgress
    return `${consoleSpan('text-blue-4', action)}${open}${consoleSpan('text-purple-3', downloaded)}${separator}${consoleSpan('text-purple-3', total)}${close}${ellipsis}`
  }

  const statusClass = /complete|launching|success/i.test(status) ? 'text-green-5' : 'text-blue-4'
  return consoleSpan(statusClass, status)
}

function parsePalworldConsole(data: string): string {
  return data.replace(/[^\r\n]+/g, (line) => parsePalworldLine(line))
}

function parsePalworldLine(line: string): string {
  const structuredLog = line.match(rePalworldStructuredLog)
  if (structuredLog !== null) {
    const [, leading = '', timestamp = '', spacing = '', level = '', gap = '', message = ''] =
      structuredLog
    const levelName = level.slice(1, -1)
    return `${leading}${consoleSpan('text-grey-6', timestamp)}${spacing}${consoleSpan(palworldLevelClass(levelName), level)}${gap}${highlightPalworldMessage(message)}`
  }

  const detailedLog = line.match(rePalworldDetailedLog)
  if (detailedLog !== null) {
    const [
      ,
      leading = '',
      timestamp = '',
      frame = '',
      spacing = '',
      category = '',
      categorySeparator = '',
      verbosity = '',
      verbositySeparator = '',
      message = '',
    ] = detailedLog
    const prefix = highlightPalworldLogPrefix(leading, timestamp, frame, spacing, category)
    return `${prefix}${categorySeparator}${consoleSpan(palworldVerbosityClass(verbosity), verbosity)}${verbositySeparator}${highlightPalworldMessage(message)}`
  }

  const categoryLog = line.match(rePalworldCategoryLog)
  if (categoryLog !== null) {
    const [
      ,
      leading = '',
      timestamp = '',
      frame = '',
      spacing = '',
      category = '',
      separator = '',
      message = '',
    ] = categoryLog
    return `${highlightPalworldLogPrefix(leading, timestamp, frame, spacing, category)}${separator}${highlightPalworldMessage(message)}`
  }

  const steamAPI = line.match(rePalworldSteamAPI)
  if (steamAPI !== null) {
    const [, leading = '', tag = '', spacing = '', message = ''] = steamAPI
    const failed = tag.toLowerCase().includes('fail')
    const className = failed ? 'text-red-5' : 'text-cyan-5'
    const highlightedMessage = failed
      ? consoleSpan(className, message)
      : highlightPalworldTrailingOK(message)
    return `${leading}${consoleSpan(className, tag)}${spacing}${highlightedMessage}`
  }

  const appID = line.match(rePalworldAppID)
  if (appID !== null) {
    const [, leading = '', label = '', separator = '', value = '', rest = ''] = appID
    return `${leading}${consoleSpan('text-cyan-5', label)}${separator}${consoleSpan('text-purple-3', value)}${rest}`
  }

  const shutdownHandler = line.match(rePalworldShutdownHandler)
  if (shutdownHandler !== null) {
    const [, leading = '', label = '', message = ''] = shutdownHandler
    return `${leading}${consoleSpan('text-cyan-5', label)}${message}`
  }

  const startupInfo = line.match(rePalworldStartupInfo)
  if (startupInfo !== null) {
    const [, leading = '', message = ''] = startupInfo
    return `${leading}${consoleSpan('text-blue-4', message)}`
  }

  const startupWarning = line.match(rePalworldStartupWarning)
  if (startupWarning !== null) {
    const [, leading = '', message = ''] = startupWarning
    return `${leading}${consoleSpan('text-yellow-5', message)}`
  }

  const steamClientOK = line.match(rePalworldSteamClientOK)
  if (steamClientOK !== null) {
    const [, leading = '', message = '', result = '', rest = ''] = steamClientOK
    return `${leading}${message}${consoleSpan('text-green-5', result)}${rest}`
  }

  return highlightPalworldMessage(line)
}

function highlightPalworldMessage(message: string): string {
  const restAccess = message.match(rePalworldRESTAccess)
  if (restAccess !== null) {
    const [, label = '', spacing = '', endpoint = '', resultSpacing = '', result = '', rest = ''] =
      restAccess
    return `${label}${spacing}${consoleSpan('text-purple-3', endpoint)}${resultSpacing}${consoleSpan('text-green-5', result)}${rest}`
  }

  const playerConnection = message.match(rePalworldPlayerConnection)
  if (playerConnection !== null) {
    const [
      ,
      player = '',
      spacing = '',
      action = '',
      userIDLabel = '',
      userID = '',
      playerIDLabel = '',
      playerID = '',
      rest = '',
    ] = playerConnection
    const eventClass = action.toLowerCase() === 'joined' ? 'text-green-5' : 'text-yellow-5'
    return `${consoleSpan(eventClass, player)}${spacing}${consoleSpan(eventClass, action)}${userIDLabel}${consoleSpan('text-purple-3', userID)}${playerIDLabel}${consoleSpan('text-purple-3', playerID)}${rest}`
  }

  const restStarted = message.match(rePalworldRESTStarted)
  if (restStarted !== null) {
    const [, label = '', spacing = '', state = '', portLabel = '', port = '', rest = ''] =
      restStarted
    return `${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-green-5', state)}${portLabel}${consoleSpan('text-purple-3', port)}${rest}`
  }

  const restStopped = message.match(rePalworldRESTStopped)
  if (restStopped !== null) {
    const [, label = '', spacing = '', state = '', rest = ''] = restStopped
    return `${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-yellow-5', state)}${rest}`
  }

  const serverStarted = message.match(rePalworldServerStarted)
  if (serverStarted !== null) {
    const [, label = '', spacing = '', endpoint = '', rest = ''] = serverStarted
    return `${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-purple-3', endpoint)}${rest}`
  }

  const gameVersion = message.match(rePalworldGameVersion)
  if (gameVersion !== null) {
    const [, label = '', spacing = '', version = '', rest = ''] = gameVersion
    return `${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-purple-3', version)}${rest}`
  }

  const requestExit = message.match(rePalworldRequestExit)
  if (requestExit !== null) {
    const [, operation = '', rest = ''] = requestExit
    return `${consoleSpan('text-yellow-5', operation)}${rest}`
  }

  const abnormalExit = message.match(rePalworldAbnormalExit)
  if (abnormalExit !== null) {
    const [, label = '', codeLabel = '', code = '', rest = ''] = abnormalExit
    return `${consoleSpan('text-red-5', label)}${codeLabel}${consoleSpan('text-red-5', code)}${rest}`
  }

  const existingFile = message.match(rePalworldExistingFile)
  if (existingFile !== null) {
    const [, label = '', spacing = '', path = ''] = existingFile
    return `${consoleSpan('text-cyan-5', label)}${spacing}${consoleSpan('text-purple-3', path)}`
  }

  const engineVersion = message.match(rePalworldEngineVersion)
  if (engineVersion !== null) {
    const [, version = '', rest = ''] = engineVersion
    return `${consoleSpan('text-purple-3', version)}${rest}`
  }

  return message
}

function highlightPalworldTrailingOK(message: string): string {
  const result = message.match(rePalworldTrailingOK)
  if (result === null) return message

  const [, prefix = '', status = '', rest = ''] = result
  return `${prefix}${consoleSpan('text-green-5', status)}${rest}`
}

function highlightPalworldLogPrefix(
  leading: string,
  timestamp: string,
  frame: string,
  spacing: string,
  category: string,
): string {
  const timestampAndFrame =
    timestamp.length === 0 ? '' : consoleSpan('text-grey-6', `${timestamp}${frame}`)
  return `${leading}${timestampAndFrame}${spacing}${consoleSpan('text-cyan-5', category)}`
}

function palworldVerbosityClass(verbosity: string): string {
  switch (verbosity.toLowerCase()) {
    case 'fatal':
    case 'error':
      return 'text-red-5'
    case 'warning':
      return 'text-yellow-5'
    case 'display':
      return 'text-green-6'
    case 'verbose':
    case 'veryverbose':
      return 'text-grey-6'
    default:
      return 'text-blue-4'
  }
}

function palworldLevelClass(level: string): string {
  switch (level.toLowerCase()) {
    case 'fatal':
    case 'error':
      return 'text-red-5'
    case 'warn':
    case 'warning':
      return 'text-yellow-5'
    case 'debug':
    case 'trace':
      return 'text-grey-6'
    default:
      return 'text-cyan-5'
  }
}

function parseVRisingConsole(data: string): string {
  data = data.replaceAll(reVRisingServer, "<span class='text-green-6'>$&</span>")
  data = data.replaceAll(reVRisingCompress, "<span class='text-orange-6'>$&</span>")
  data = data.replaceAll(reVRisingPersistence, "<span class='text-blue-6'>$&</span>")
  data = data.replaceAll(reVRisingFinishedSaving, "<span class='text-purple-4'>$&</span>")
  return data
}

function parseDefaultConsole(data: string): string {
  data = data.replaceAll(reInfo, "<span class='text-green-6'>$&</span>")
  data = data.replaceAll(reWarn, "<span class='text-yellow-5'>$&</span>")
  data = data.replaceAll(reError, "<span class='text-red-5'>$&</span>")
  data = data.replace(reURIMatch, "<a class='console-url' href='$&' target='_blank'>$&</a>")
  return data
}

function parse7DaysToDieConsole(data: string): string {
  data = data.replaceAll(re7dTimestamp, `<span class='text-grey-6'>[$1 $2]</span> $4`)
  data = data.replaceAll(re7dInfoLine, "<span class='text-green-6'>$1$2</span>")
  data = data.replaceAll('[Steamworks.NET]', '<span class="text-yellow-7">[Steamworks.NET]</span>')
  data = data.replaceAll(
    re7dOS,
    "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>",
  )
  data = data.replaceAll(
    re7dCPU,
    "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>",
  )
  data = data.replaceAll(
    re7dRAM,
    "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>",
  )
  data = data.replaceAll(
    re7dGPU,
    "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>",
  )
  data = data.replaceAll(re7dSystemInfo, "$1<span class='text-orange-4'>$2</span>")
  data = data.replaceAll(
    re7dVersion,
    "<span class='text-orange-4'>$1</span>$2<span class='text-cyan-5'>$3</span>",
  )
  data = data.replaceAll(re7dHelpCommands, "<span class='text-purple-3'> $1</span> =&gt;")
  data = data.replaceAll('[MODS]', '<span class="text-yellow-7">[MODS]</span>')
  data = data.replaceAll('[EAC]', '<span class="text-red-3">[EAC]</span>')
  return data
}

function parseMinecraftConsole(data: string): string {
  const playerJoin = execRegex(reMcPlayerJoin, data)
  const playerLeave = execRegex(reMcPlayerLeave, data)

  data = data.replace(reURIMatch, "<a class='console-url' href='$&' target='_blank'>$&</a>")

  if (playerJoin && playerJoin.length > 1) {
    const username = playerJoin[1]
    const uid = playerJoin[2]
    if (username !== undefined && uid !== undefined) {
      minecraftPlayerMap.set(username, {
        username,
        color: StringToColor(username),
        uid,
      })
    }
  }
  minecraftPlayerMap.forEach((player: MinecraftPlayer, username: string) => {
    data = data.replaceAll(
      username,
      `<a style="text-decoration: ${player.color} underline" href="https://sessionserver.mojang.com/session/minecraft/profile/${player.uid}" target="_blank"><span style="color: ${player.color}">${username}</span></a>`,
    )
  })
  if (playerLeave) {
    const username = playerLeave[1]
    if (username !== undefined) {
      minecraftPlayerMap.delete(username)
    }
  }

  data = data.replace(reMcServerInfo, "$1<span class='text-green-6'>$2</span>")
  data = data.replace(reMcServerWarn, "$1<span class='text-yellow-5'>$2</span>")
  data = data.replace(reMcServerError, "$1<span class='text-red-5'>$2</span>")
  data = data.replace(reMcTimestamp, "<span class='text-grey-6'>$&</span>")
  return data
}
