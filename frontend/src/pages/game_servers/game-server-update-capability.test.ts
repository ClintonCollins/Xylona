import { describe, expect, it } from 'vitest'

import { CommandProcessor, type GameServer } from '@/proto/shared_pb'
import { canShowUpdateButton } from './game-server-update-capability'

function buildGameServer(
  gameOverrides: Partial<GameServer['game']> = {},
  serverOverrides: Partial<GameServer> = {},
): GameServer {
  return {
    gameId: 'other-game',
    serverSoftware: '',
    game: {
      linuxUpdateCommand: '',
      linuxUpdateCommandProcessor: CommandProcessor.DIRECT,
      windowsUpdateCommand: '',
      windowsUpdateCommandProcessor: CommandProcessor.DIRECT,
      serverSoftware: '',
      ...gameOverrides,
    },
    ...serverOverrides,
  } as GameServer
}

describe('canShowUpdateButton', () => {
  it('returns false when no update command is configured and neither processor is internal', () => {
    expect(canShowUpdateButton(buildGameServer())).toBe(false)
  })

  it('returns true when a linux update command is configured', () => {
    expect(
      canShowUpdateButton(buildGameServer({ linuxUpdateCommand: 'steamcmd +app_update' })),
    ).toBe(true)
  })

  it('returns true when the linux update processor is internal', () => {
    expect(
      canShowUpdateButton(
        buildGameServer({
          linuxUpdateCommandProcessor: CommandProcessor.XYLONA_INTERNAL,
        }),
      ),
    ).toBe(true)
  })

  it('returns true for minecraft vanilla when the active software is provider-backed via legacy mapping', () => {
    expect(
      canShowUpdateButton(
        buildGameServer(
          {
            serverSoftware: JSON.stringify([{ id: 'vanilla', name: 'Vanilla', jar_source: null }]),
          },
          {
            gameId: 'minecraft',
            serverSoftware: 'vanilla',
          },
        ),
      ),
    ).toBe(true)
  })

  it('returns true for minecraft provider-backed variants', () => {
    expect(
      canShowUpdateButton(
        buildGameServer(
          {
            serverSoftware: JSON.stringify([{ id: 'paper', name: 'Paper', jar_source: 'papermc' }]),
          },
          {
            gameId: 'minecraft',
            serverSoftware: 'paper',
          },
        ),
      ),
    ).toBe(true)
  })

  it('returns false for unsupported minecraft variants', () => {
    expect(
      canShowUpdateButton(
        buildGameServer(
          {
            serverSoftware: JSON.stringify([{ id: 'fabric', name: 'Fabric', jar_source: null }]),
          },
          {
            gameId: 'minecraft',
            serverSoftware: 'fabric',
          },
        ),
      ),
    ).toBe(false)
  })
})
