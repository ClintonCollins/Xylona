import { describe, expect, it } from 'vitest'

import type { GameServer } from '@/proto/shared_pb'
import { canShowUpdateButton } from './game-server-update-capability'

function buildGameServer(serverOverrides: Partial<GameServer> = {}): GameServer {
  return {
    gameId: 'other-game',
    resolvedHasUpdate: false,
    ...serverOverrides,
  } as GameServer
}

describe('canShowUpdateButton', () => {
  it('returns false when the resolved update capability is disabled', () => {
    expect(canShowUpdateButton(buildGameServer())).toBe(false)
  })

  it('returns true when the resolved update capability is enabled', () => {
    expect(canShowUpdateButton(buildGameServer({ resolvedHasUpdate: true }))).toBe(true)
  })
})
