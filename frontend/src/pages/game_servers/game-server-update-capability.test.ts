import { describe, expect, it } from 'vitest'

import { type GameServer, UpdateProviderKind } from '@/proto/shared_pb'
import { canShowUpdateButton } from './game-server-update-capability'

function buildGameServer(serverOverrides: Partial<GameServer> = {}): GameServer {
  return {
    gameId: 'other-game',
    resolvedHasUpdate: false,
    ...serverOverrides,
  } as GameServer
}

describe('canShowUpdateButton', () => {
  const cases: { name: string; server: GameServer; expected: boolean }[] = [
    {
      name: 'no update provider and no pending update',
      server: buildGameServer(),
      expected: false,
    },
    {
      name: 'provider kind NONE and no pending update',
      server: buildGameServer({
        resolvedUpdateProvider: { kind: UpdateProviderKind.NONE },
      } as Partial<GameServer>),
      expected: false,
    },
    {
      name: 'pending update without a resolved provider',
      server: buildGameServer({ resolvedHasUpdate: true }),
      expected: true,
    },
    {
      name: 'steamcmd provider with no pending update',
      server: buildGameServer({
        resolvedUpdateProvider: { kind: UpdateProviderKind.STEAMCMD },
      } as Partial<GameServer>),
      expected: true,
    },
    {
      name: 'papermc provider with no pending update',
      server: buildGameServer({
        resolvedUpdateProvider: { kind: UpdateProviderKind.PAPERMC },
      } as Partial<GameServer>),
      expected: true,
    },
    {
      name: 'mojang provider with no pending update',
      server: buildGameServer({
        resolvedUpdateProvider: { kind: UpdateProviderKind.MOJANG },
      } as Partial<GameServer>),
      expected: true,
    },
    {
      name: 'command provider with no pending update',
      server: buildGameServer({
        resolvedUpdateProvider: { kind: UpdateProviderKind.COMMAND },
      } as Partial<GameServer>),
      expected: true,
    },
  ]

  it.each(cases)('$name', ({ server, expected }) => {
    expect(canShowUpdateButton(server)).toBe(expected)
  })
})
