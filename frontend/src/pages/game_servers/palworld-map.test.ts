import { describe, expect, it } from 'vitest'
import { PalworldMapActorKind, type PalworldMapActor } from '@/proto/xylona_pb'
import {
  assignPalworldBaseWorkers,
  buildPalworldMapRenderPlan,
  estimatePalworldLabelWidth,
  filterPalworldMapActors,
  formatPalworldFacing,
  formatPalworldCoordinate,
  formatPalworldUptime,
  initialPalworldMapVisibility,
  palworldLabelMetrics,
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

  it('orders actors by code unit so the plan does not depend on the viewer locale', () => {
    const localeSensitiveKeys = ['b-1', 'B-1', 'a_2', 'A_2']
    const fixture = localeSensitiveKeys.map((key) => ({ ...playerActor, key }))

    const plan = buildPalworldMapRenderPlan(fixture, { zoom: 6 })

    expect(plan.actors.map((actor) => actor.key)).toEqual(['A_2', 'B-1', 'a_2', 'b-1'])
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

function baseAt(
  key: string,
  name: string,
  locationX: number,
  guildKey = 'guild-1',
): PalworldMapActor {
  return {
    ...playerActor,
    key,
    kind: PalworldMapActorKind.BASE,
    name,
    locationX,
    locationY: 0,
    guildKey,
    guildName: 'Skyforge',
  }
}

function workerAt(key: string, locationX: number, guildKey = 'guild-1'): PalworldMapActor {
  return {
    ...playerActor,
    key,
    kind: PalworldMapActorKind.BASE_WORKER,
    name: 'Anubis',
    locationX,
    locationY: 0,
    guildKey,
    guildName: 'Skyforge',
  }
}

describe('Palworld base worker assignment', () => {
  it('assigns every worker to the nearest base within its own guild', () => {
    const northBase = baseAt('base-north', 'North Camp', 0)
    const southBase = baseAt('base-south', 'South Camp', 1_000)
    const rivalBase = baseAt('base-rival', 'Rival Camp', 10, 'guild-2')
    const fixture = [
      workerAt('worker-far', 990),
      workerAt('worker-near', 20),
      workerAt('worker-rival', 0, 'guild-2'),
      northBase,
      southBase,
      rivalBase,
    ]

    const assignment = assignPalworldBaseWorkers(fixture)
    const reversed = assignPalworldBaseWorkers([...fixture].reverse())

    expect(assignment.byBase.get('base-north')?.map((actor) => actor.key)).toEqual(['worker-near'])
    expect(assignment.byBase.get('base-south')?.map((actor) => actor.key)).toEqual(['worker-far'])
    // A worker never crosses guilds, even when a rival base is closer.
    expect(assignment.byBase.get('base-rival')?.map((actor) => actor.key)).toEqual(['worker-rival'])
    expect(assignment.unassigned).toEqual([])
    expect(reversed.byBase.get('base-north')).toEqual(assignment.byBase.get('base-north'))
  })

  it('leaves workers whose guild owns no base unassigned', () => {
    const orphan = workerAt('worker-orphan', 5, 'guild-empty')
    const guildless: PalworldMapActor = {
      ...workerAt('worker-guildless', 5),
      guildKey: '',
      guildName: '',
    }

    const assignment = assignPalworldBaseWorkers([
      baseAt('base-north', 'North', 0),
      orphan,
      guildless,
    ])

    expect(assignment.byBase.get('base-north') ?? []).toEqual([])
    expect(assignment.unassigned.map((actor) => actor.key)).toEqual([
      'worker-guildless',
      'worker-orphan',
    ])
  })
})

describe('Palworld map label decluttering', () => {
  const plan = (actors: PalworldMapActor[], options = {}) =>
    buildPalworldMapRenderPlan(actors, { zoom: 6, ...options })

  it('keeps every label when pills do not overlap', () => {
    const result = plan([baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Beta', 500)])

    expect([...result.compactActorKeys]).toEqual([])
  })

  it('keeps one label per overlapping stack and picks the winner deterministically', () => {
    const stack = [
      baseAt('base-3', 'Gamma', 0),
      baseAt('base-1', 'Alpha', 0),
      baseAt('base-2', 'Beta', 0),
    ]

    const result = plan(stack)
    const reversed = plan([...stack].reverse())

    expect([...result.compactActorKeys].toSorted()).toEqual(['base-2', 'base-3'])
    expect([...reversed.compactActorKeys].toSorted()).toEqual(['base-2', 'base-3'])
  })

  const playerAtOrigin: PalworldMapActor = { ...playerActor, locationX: 0, locationY: 0 }

  it.each([
    {
      name: 'a player outranks a base on the same spot',
      actors: [playerAtOrigin, baseAt('base-1', 'Alpha', 0)],
      options: {},
      expected: ['base-1'],
    },
    {
      name: 'the selected actor outranks everything',
      actors: [playerAtOrigin, baseAt('base-1', 'Alpha', 0)],
      options: { selectedActorKey: 'base-1' },
      expected: ['player-1'],
    },
    {
      name: 'a protected actor outranks an unprotected peer',
      actors: [baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Beta', 0)],
      options: { protectedActorKeys: new Set(['base-2']) },
      expected: ['base-1'],
    },
  ])('$name', ({ actors: contenders, options, expected }) => {
    const result = plan(contenders, options)

    expect([...result.compactActorKeys].toSorted()).toEqual(expected)
  })

  it('requires extra clearance before a hidden label comes back', () => {
    // 88px apart clears the collision padding but not the hysteresis margin.
    const stack = [baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Alpha', 88)]

    const settled = plan(stack)
    const recovering = plan(stack, { previousCompactActorKeys: new Set(['base-2']) })

    expect([...settled.compactActorKeys]).toEqual([])
    expect([...recovering.compactActorKeys]).toEqual(['base-2'])
  })

  it('estimates label width from the name and clamps it to the CSS bound', () => {
    expect(estimatePalworldLabelWidth('')).toBe(palworldLabelMetrics.avgCharWidth)
    expect(estimatePalworldLabelWidth('Alpha')).toBe(5 * palworldLabelMetrics.avgCharWidth)
    expect(estimatePalworldLabelWidth('x'.repeat(200))).toBe(palworldLabelMetrics.maxLabelWidth)
  })
})
