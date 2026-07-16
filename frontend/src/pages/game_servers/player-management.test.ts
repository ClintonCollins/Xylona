import { describe, expect, it } from 'vitest'
import { GameServerPlayerAction } from '@/proto/xylona_pb'
import {
  getPlayerActionDefinition,
  getQuickPlayerActionDefinitions,
  getSupportedPlayerActionDefinitions,
} from './player-management'

describe('player management action definitions', () => {
  it.each([
    { action: GameServerPlayerAction.KICK, label: 'Kick', reasonAllowed: true },
    { action: GameServerPlayerAction.BAN, label: 'Ban', reasonAllowed: true },
    { action: GameServerPlayerAction.UNBAN, label: 'Unban', reasonAllowed: false },
    {
      action: GameServerPlayerAction.ALLOWLIST_ADD,
      label: 'Add to allowlist',
      reasonAllowed: false,
    },
    {
      action: GameServerPlayerAction.ALLOWLIST_REMOVE,
      label: 'Remove from allowlist',
      reasonAllowed: false,
    },
  ])('defines $label', ({ action, label, reasonAllowed }) => {
    expect(getPlayerActionDefinition(action)).toMatchObject({ label, reasonAllowed })
  })

  it('ignores unspecified and unknown actions', () => {
    expect(getPlayerActionDefinition(GameServerPlayerAction.UNSPECIFIED)).toBeNull()
    expect(getPlayerActionDefinition(999 as GameServerPlayerAction)).toBeNull()
  })

  it('keeps the node-provided action order', () => {
    const definitions = getSupportedPlayerActionDefinitions([
      GameServerPlayerAction.UNBAN,
      GameServerPlayerAction.KICK,
    ])
    expect(definitions.map((definition) => definition.label)).toEqual(['Unban', 'Kick'])
  })

  it('limits quick actions to kick and ban', () => {
    const definitions = getQuickPlayerActionDefinitions([
      GameServerPlayerAction.KICK,
      GameServerPlayerAction.UNBAN,
      GameServerPlayerAction.BAN,
      GameServerPlayerAction.ALLOWLIST_ADD,
    ])
    expect(definitions.map((definition) => definition.action)).toEqual([
      GameServerPlayerAction.KICK,
      GameServerPlayerAction.BAN,
    ])
  })
})
