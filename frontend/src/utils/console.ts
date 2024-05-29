import {StringToColor} from "src/utils/shared";

const reURIMatch = /(?:(?:https?|ftp|file):\/\/|www\.|ftp\.)(?:\([-A-Z0-9+&@#\/%=~_|$?!:,.]*\)|[-A-Z0-9+&@#\/%=~_|$?!:,.])*(?:\([-A-Z0-9+&@#\/%=~_|$?!:,.]*\)|[A-Z0-9+&@#\/%=~_|$])/igm
const reServerStop = /Server stopped.+$/gmi
const reExitStatus = /^exit status [1-9]|^exit status 0.+$/gmi
const reInfo = /^INFO|INF/gm
const reWarn = /^WARNING|WARN|WRN/gm
const reError = /ERROR/gm
const reXylonaMessage = /\[(\d+-\d+-\d+\s\d+:\d+:\d+)]\s\[(Xylona)]/gm
const reMinecraftVersion = /Starting\sminecraft\sserver\sversion\s(.+)$/gmi


type MinecraftPlayer = {
    username: string
    color: string
    uid: string
}

let minecraftPlayerMap: Map<string, MinecraftPlayer> = new Map<string, MinecraftPlayer>

export function parseConsole(game: string, data: string): string {
    data = data.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
    data = data.replace(reServerStop, "<span class='text-red-5'>$&</span>")
    data = data.replace(reExitStatus, "<span class='text-red-5'>$&</span>")
    switch (game.toLowerCase()) {
        case "minecraft":
            data = parseMinecraftConsole(data)
            break
        case "7_days_to_die":
            data = parse7DaysToDieConsole(data)
            break
        case "v-rising":
            data = parseVRisingConsole(data)
            break
        default:
            data = parseDefaultConsole(data)
            break
    }
    data = data.replaceAll(reError, "<span class='text-red-5'>$&</span>")
    data = data.replaceAll(reWarn, "<span class='text-yellow-5'>$&</span>")
    data = data.replaceAll(reXylonaMessage, "<span class='text-grey-6'>[$1]</span> <span class='text-cyan-7'>[$2]</span>")
    return data
}

function parseVRisingConsole(data: string): string {
    const reServer = /^\[Server]/gm
    const reCompress = /^\[CompressModificationIdsOnLoadSystem]/gm
    const rePersistence = /^PersistenceV2/gm
    const reFinishedSaving = /(?<=Finished Saving to\s').+(?='.)/gm
    data = data.replaceAll(reServer, "<span class='text-green-6'>$&</span>")
    data = data.replaceAll(reCompress, "<span class='text-orange-6'>$&</span>")
    data = data.replaceAll(rePersistence, "<span class='text-blue-6'>$&</span>")
    data = data.replaceAll(reFinishedSaving, "<span class='text-purple-4'>$&</span>")
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
    const reTimestamp = /^(\d+-\d+-\d+)T(\d+:\d+:\d+)( \d+.\d+)\s(INF)/gmi
    const reInfo = /^(.+\s)(INF\s)/gmi
    const reOS = /^(.+)(OS):(.+)$/gmi
    const reCPU = /^(.+)(CPU):(.+)$/gmi
    const reRAM = /^(.+)(RAM):(.+)$/gmi
    const reGPU = /^(.+)(GPU):(.+)$/gmi
    const reSystemInfo = /^(.+)(System information)/gmi
    const reVersion = /(Version)(:\s)(.+)$/gmi
    const reHelpCommands = /^\s(.+)\s=&gt;/gmi

    data = data.replaceAll(reTimestamp, `<span class='text-grey-6'>[$1 $2]</span> $4`)
    data = data.replaceAll(reInfo, "<span class='text-green-6'>$1$2</span>")
    data = data.replaceAll('[Steamworks.NET]', '<span class="text-yellow-7">[Steamworks.NET]</span>')
    data = data.replaceAll(reOS, "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>")
    data = data.replaceAll(reCPU, "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>")
    data = data.replaceAll(reRAM, "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>")
    data = data.replaceAll(reGPU, "$1<span class='text-orange-4'>$2</span>:<span class='text-cyan-5'>$3</span>")
    data = data.replaceAll(reSystemInfo, "$1<span class='text-orange-4'>$2</span>")
    data = data.replaceAll(reVersion, "<span class='text-orange-4'>$1</span>$2<span class='text-cyan-5'>$3</span>")
    data = data.replaceAll(reHelpCommands, "<span class='text-purple-3'> $1</span> =&gt;")
    data = data.replaceAll('[MODS]', '<span class="text-yellow-7">[MODS]</span>')
    data = data.replaceAll('[EAC]', '<span class="text-red-3">[EAC]</span>')
    return data
}

function parseMinecraftConsole(data: string): string {
    const reServerInfo = /(^.+)(\[.+INFO])/gm
    const reServerWarn = /(^.+)(\[.+WARN])/gm
    const reServerError = /(^.+)(\[.+ERROR])/gm
    const reTimestamp = /\[\d+:\d+:\d+]/gmi
    const rePlayerJoin = /^\[.+]\s\[User Authenticator.+]:\sUUID of player\s(.+)\sis\s(.+)/gmi
    const rePlayerLeave = /^\[.+]\s\[.+]:\s(.+)\sleft the game/gmi

    const playerJoin = rePlayerJoin.exec(data)
    const playerLeave = rePlayerLeave.exec(data)

    data = data.replace(reURIMatch, "<a class='console-url' href='$&' target='_blank'>$&</a>")

    if (playerJoin && playerJoin.length > 1) {
        console.log(playerJoin[1])
        minecraftPlayerMap.set(playerJoin[1], {username: playerJoin[1], color: StringToColor(playerJoin[1]), uid: playerJoin[2]})
    }
    minecraftPlayerMap.forEach((player: MinecraftPlayer, username: string) => {
        console.log(player, player.color)
        data = data.replaceAll(username, `<a style="text-decoration: ${player.color} underline" href="https://sessionserver.mojang.com/session/minecraft/profile/${player.uid}" target="_blank"><span style="color: ${player.color}">${username}</span></a>`)
    })
    if (playerLeave) {
        minecraftPlayerMap.delete(playerLeave[1])
    }

    data = data.replace(reServerInfo, "$1<span class='text-green-6'>$2</span>")
    data = data.replace(reServerWarn, "$1<span class='text-yellow-5'>$2</span>")
    data = data.replace(reServerError, "$1<span class='text-red-5'>$2</span>")
    data = data.replace(reTimestamp, "<span class='text-grey-6'>$&</span>")
    return data
}
