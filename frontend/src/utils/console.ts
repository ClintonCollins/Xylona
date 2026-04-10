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
  data = data.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  data = data.replace(reServerStop, "<span class='text-red-5'>$&</span>")
  data = data.replace(reExitStatus, "<span class='text-red-5'>$&</span>")
  switch (game.toLowerCase()) {
    case 'minecraft':
      data = parseMinecraftConsole(data)
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
