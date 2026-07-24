import { describe, expect, it } from 'vitest'
import { PalworldMapActorKind, type PalworldMapActor } from '@/proto/xylona_pb'
import {
  buildPalworldMapRenderPlan,
  filterPalworldMapActors,
  formatPalworldFacing,
  formatPalworldCoordinate,
  formatPalworldUptime,
  initialPalworldMapVisibility,
  palworldMapCategories,
  palworldMapGuildKey,
} from './palworld-map'

const playerActor: PalworldMapActor = {
  $typeName: 'xylona.PalworldMapActor',
  key: 'player-1',
  kind: PalworldMapActorKind.PLAYER,
  name: 'Alex',
  guildKey: '',
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
}

const wildActor: PalworldMapActor = {
  $typeName: 'xylona.PalworldMapActor',
  key: 'wild-1',
  kind: PalworldMapActorKind.WILD_PAL,
  name: 'Lamball',
  guildKey: '',
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
}

const actors = [playerActor, wildActor]

describe('Palworld map actor helpers', () => {
  it('filters by enabled layers and exact actor metadata', () => {
    const visibility = initialPalworldMapVisibility()
    expect(palworldMapCategories.every((category) => visibility[category.kind])).toBe(true)
    expect(filterPalworldMapActors(actors, visibility, '')).toEqual(actors)

    visibility[PalworldMapActorKind.WILD_PAL] = false
    expect(filterPalworldMapActors(actors, visibility, '')).toEqual([actors[0]])

    visibility[PalworldMapActorKind.WILD_PAL] = true
    expect(filterPalworldMapActors(actors, visibility, 'sheepball')).toEqual([actors[1]])
    expect(filterPalworldMapActors(actors, visibility, 'skyforge')).toEqual([actors[0]])
  })

  it('formats exact coordinates consistently', () => {
    expect(formatPalworldCoordinate(123.456)).toBe('123.46')
    expect(formatPalworldCoordinate(Number.NaN)).toBe('—')
  })

  it('formats compact operational values and stable guild identities', () => {
    expect(formatPalworldFacing(-45)).toBe('NW 315°')
    expect(formatPalworldFacing(91)).toBe('E 91°')
    expect(formatPalworldUptime(90)).toBe('1m')
    expect(formatPalworldUptime(93_900n)).toBe('1d 2h')
    expect(palworldMapGuildKey(playerActor)).toBe('name:skyforge')

    const actorWithGuildKey = Object.assign({}, playerActor, { guildKey: 'guild-hash' })
    expect(palworldMapGuildKey(actorWithGuildKey)).toBe('key:guild-hash')
  })

  it('builds deterministic category-specific clusters while preserving priority actors', () => {
    const clusteredActors = [
      playerActor,
      wildActor,
      {
        ...wildActor,
        key: 'wild-2',
        locationX: 32,
        locationY: 41,
      },
      {
        ...wildActor,
        key: 'worker-1',
        kind: PalworldMapActorKind.BASE_WORKER,
        locationX: 31,
        locationY: 41,
      },
    ]
    const options = {
      zoom: 2,
      clusterCellSize: 100,
      protectedActorKeys: new Set(['wild-2']),
    }

    const first = buildPalworldMapRenderPlan(clusteredActors, options)
    const second = buildPalworldMapRenderPlan([...clusteredActors].reverse(), options)

    expect(first).toEqual(second)
    expect(first.actors.map((actor) => actor.key)).toEqual([
      'player-1',
      'wild-1',
      'wild-2',
      'worker-1',
    ])
    expect(first.clusters).toEqual([])

    const clustered = buildPalworldMapRenderPlan(clusteredActors, {
      zoom: 2,
      clusterCellSize: 100,
    })
    expect(clustered.actors.map((actor) => actor.key)).toEqual(['player-1', 'worker-1'])
    expect(clustered.clusters).toMatchObject([
      {
        key: `cluster:${PalworldMapActorKind.WILD_PAL}:0:0`,
        kind: PalworldMapActorKind.WILD_PAL,
        count: 2,
      },
    ])
    expect(clustered.aggregatedActorCount).toBe(2)
  })

  it('bounds a 25k low-zoom render plan without mutating its source', () => {
    const fixture = Array.from(
      { length: 25_000 },
      (_, index): PalworldMapActor => ({
        ...wildActor,
        key: `wild-${index.toString().padStart(5, '0')}`,
        locationX: index % 500,
        locationY: Math.floor(index / 500),
      }),
    )
    const sourceOrder = fixture.map((actor) => actor.key)

    const plan = buildPalworldMapRenderPlan(fixture, {
      zoom: 1,
      clusterCellSize: 50,
      bounds: { minX: 0, minY: 0, maxX: 499, maxY: 49 },
    })

    expect(plan.actors).toHaveLength(0)
    expect(plan.clusters.length).toBeLessThanOrEqual(10)
    expect(plan.aggregatedActorCount).toBe(25_000)
    expect(fixture.map((actor) => actor.key)).toEqual(sourceOrder)
  })

  it('keeps dense high-zoom views aggregated above the individual marker limit', () => {
    const fixture = Array.from(
      { length: 1_501 },
      (_, index): PalworldMapActor => ({
        ...wildActor,
        key: `wild-${index.toString().padStart(4, '0')}`,
        locationX: index % 100,
        locationY: Math.floor(index / 100),
      }),
    )

    const plan = buildPalworldMapRenderPlan(fixture, {
      zoom: 5,
      clusterCellSize: 25,
      maxIndividualActors: 1_500,
    })

    expect(plan.actors.length).toBeLessThanOrEqual(16)
    expect(plan.clusters.length).toBeGreaterThan(0)
    expect(plan.aggregatedActorCount).toBeGreaterThan(1_450)
  })
})
