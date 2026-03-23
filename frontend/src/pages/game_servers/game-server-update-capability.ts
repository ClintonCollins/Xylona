import { CommandProcessor, type GameServer } from '@/proto/shared_pb'

interface ServerSoftwareConfig {
  id?: string
  jar_source?: string | null
}

function minecraftProviderId(gameServer: GameServer): string {
  const activeSoftwareId = gameServer.serverSoftware.trim().toLowerCase()
  if (activeSoftwareId === '') return ''

  const serverSoftwareJson = gameServer.game?.serverSoftware ?? ''
  if (serverSoftwareJson.trim() === '') return ''

  try {
    const parsed = JSON.parse(serverSoftwareJson) as ServerSoftwareConfig[]
    const activeSoftware = parsed.find(
      (software) => software.id?.toLowerCase() === activeSoftwareId,
    )
    if (!activeSoftware) return ''

    const jarSource = activeSoftware.jar_source?.trim().toLowerCase() ?? ''
    if (jarSource !== '') return jarSource
    if (activeSoftwareId === 'vanilla') return 'mojang'
    return ''
  } catch {
    return ''
  }
}

export function canShowUpdateButton(gameServer: GameServer): boolean {
  const game = gameServer.game
  if (!game) return false

  if (gameServer.gameId === 'minecraft') {
    return minecraftProviderId(gameServer) !== ''
  }

  return (
    game.linuxUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL ||
    game.windowsUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL ||
    game.linuxUpdateCommand.trim() !== '' ||
    game.windowsUpdateCommand.trim() !== ''
  )
}
