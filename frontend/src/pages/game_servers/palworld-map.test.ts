import { describe, expect, it } from 'vitest'
import { PalworldMapActorKind, type PalworldMapActor } from '@/proto/xylona_pb'
import {
  filterPalworldMapActors,
  formatPalworldCoordinate,
  initialPalworldMapVisibility,
} from './palworld-map'

const actors: PalworldMapActor[] = [
  {
    $typeName: 'xylona.PalworldMapActor',
    key: 'player-1',
    kind: PalworldMapActorKind.PLAYER,
    name: 'Alex',
    guildName: 'Skyforge',
    trainerName: '',
    className: '',
    locationX: 10,
    locationY: 20,
    locationZ: 0,
    rotationZ: 0,
    level: 20,
    hp: 0,
    maxHp: 0,
    action: '',
    aiAction: '',
    active: true,
  },
  {
    $typeName: 'xylona.PalworldMapActor',
    key: 'wild-1',
    kind: PalworldMapActorKind.WILD_PAL,
    name: 'Lamball',
    guildName: '',
    trainerName: '',
    className: 'Pal_SheepBall',
    locationX: 30,
    locationY: 40,
    locationZ: 0,
    rotationZ: 0,
    level: 4,
    hp: 0,
    maxHp: 0,
    action: '',
    aiAction: '',
    active: true,
  },
]

describe('Palworld map actor helpers', () => {
  it('filters by enabled layers and exact actor metadata', () => {
    const visibility = initialPalworldMapVisibility()
    expect(filterPalworldMapActors(actors, visibility, '')).toEqual([actors[0]])

    visibility[PalworldMapActorKind.WILD_PAL] = true
    expect(filterPalworldMapActors(actors, visibility, 'sheepball')).toEqual([actors[1]])
    expect(filterPalworldMapActors(actors, visibility, 'skyforge')).toEqual([actors[0]])
  })

  it('formats exact coordinates consistently', () => {
    expect(formatPalworldCoordinate(123.456)).toBe('123.46')
    expect(formatPalworldCoordinate(Number.NaN)).toBe('—')
  })
})
