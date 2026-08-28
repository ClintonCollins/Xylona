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
  palworldDotMetrics,
  palworldLabelMetrics,
  palworldMapCategories,
  palworldMapCategory,
  palworldMapDotRadius,
  palworldMapFocusZoom,
  palworldMapGuildKey,
  palworldMapIconSize,
  palworldMapMaxZoom,
  palworldMapUsesIconMarkers,
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
    const fixture = Array.from({ length: 25_000 }, (_, index): PalworldMapActor => ({
      ...wildActor,
      key: `wild-${index.toString().padStart(5, '0')}`,
      locationX: index % 500,
      locationY: Math.floor(index / 500),
    }))
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
    // Spread apart so ordering is the only thing under test; co-located pills
    // merge into a single chip and would leave nothing to order.
    const fixture = localeSensitiveKeys.map((key, index) => ({
      ...playerActor,
      key,
      locationX: index * 300,
    }))

    const plan = buildPalworldMapRenderPlan(fixture, { zoom: 6 })

    expect(plan.actors.map((actor) => actor.key)).toEqual(['A_2', 'B-1', 'a_2', 'b-1'])
  })

  it('keeps dense high-zoom views aggregated above the individual marker limit', () => {
    const fixture = Array.from({ length: 1_501 }, (_, index): PalworldMapActor => ({
      ...wildActor,
      key: `wild-${index.toString().padStart(4, '0')}`,
      locationX: index % 100,
      locationY: Math.floor(index / 100),
    }))

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

  it('collapses a label whose name collides but whose icon still has room', () => {
    // 60px apart clears the 34px icon boxes but not the full 'Alpha' pill.
    const result = plan([baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Beta', 60)])

    expect([...result.compactActorKeys]).toEqual(['base-2'])
    expect(result.clusters).toEqual([])
    expect(result.actors.map((actor) => actor.key)).toEqual(['base-1', 'base-2'])
  })

  it('merges a stack whose icons also collide and picks the anchor deterministically', () => {
    const stack = [
      baseAt('base-3', 'Gamma', 0),
      baseAt('base-1', 'Alpha', 0),
      baseAt('base-2', 'Beta', 0),
    ]

    const result = plan(stack)
    const reversed = plan([...stack].reverse())

    expect(result).toEqual(reversed)
    expect(result.actors).toEqual([])
    expect(result.clusters).toMatchObject([
      {
        key: 'merge:base-1',
        kind: PalworldMapActorKind.BASE,
        count: 3,
        labelMerge: true,
      },
    ])
    expect(result.aggregatedActorCount).toBe(3)
  })

  it('reports its members so a stack that cannot be zoomed apart stays reachable', () => {
    const result = plan([
      baseAt('base-2', 'Beta', 0),
      baseAt('base-1', 'Alpha', 0),
      baseAt('base-3', 'Gamma', 0),
    ])

    expect(result.clusters[0]?.actorKeys).toEqual(['base-1', 'base-2', 'base-3'])
    expect(result.mergedActorKeys).toEqual(new Set(['base-1', 'base-2', 'base-3']))
  })

  it('requires extra clearance before a merged stack comes apart', () => {
    // 44px apart clears the icon boxes but not the merge hysteresis margin.
    const stack = [baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Beta', 44)]

    const settled = plan(stack)
    const recovering = plan(stack, { previousMergedActorKeys: new Set(['base-2']) })

    expect(settled.clusters).toEqual([])
    expect(recovering.clusters).toMatchObject([{ key: 'merge:base-1', count: 2 }])
  })

  // Players are what an operational map is watched for, so they collapse to an
  // icon rather than disappearing into an anonymous count.
  it('collapses co-located players instead of folding them into a chip', () => {
    const result = plan([
      { ...playerActor, key: 'player-a', name: 'Alex', locationX: 0, locationY: 0 },
      { ...playerActor, key: 'player-b', name: 'Sam', locationX: 0, locationY: 0 },
    ])

    expect(result.clusters).toEqual([])
    expect(result.actors.map((actor) => actor.key)).toEqual(['player-a', 'player-b'])
    expect([...result.compactActorKeys]).toEqual(['player-b'])
  })

  it('never merges across kinds, so a base stacked on a player only collapses', () => {
    const result = plan([{ ...playerActor, locationX: 0, locationY: 0 }, baseAt('base-1', 'A', 0)])

    expect(result.clusters).toEqual([])
    expect([...result.compactActorKeys]).toEqual(['base-1'])
  })

  it.each([
    { name: 'the selected actor', options: { selectedActorKey: 'base-1' } },
    { name: 'a protected actor', options: { protectedActorKeys: new Set(['base-1']) } },
  ])('never merges $name away', ({ options }) => {
    const stack = [baseAt('base-1', 'Alpha', 0), baseAt('base-2', 'Beta', 0)]

    const result = plan(stack, options)

    expect(result.clusters).toEqual([])
    expect(result.actors.map((actor) => actor.key)).toEqual(['base-1', 'base-2'])
    expect([...result.compactActorKeys]).toEqual(['base-2'])
  })

  it('merges by screen cell instead of pairwise once the candidate guard trips', () => {
    const crowd = Array.from({ length: palworldLabelMetrics.maxLabelCandidates + 1 }, (_, index) =>
      baseAt(`base-${index.toString().padStart(4, '0')}`, `Camp ${index}`, index % 2),
    )

    const result = plan(crowd)

    expect(result.clusters.length).toBeGreaterThan(0)
    expect(result.clusters.every((cluster) => cluster.labelMerge)).toBe(true)
    expect(result.aggregatedActorCount).toBe(crowd.length)
    expect(result.actors).toEqual([])
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

describe('Palworld map zoom range', () => {
  // The built-in tile layers stop at 4 and the coordinate grid at 6; both gain
  // the same three upscaled levels so behaviour does not depend on the source.
  it.each([
    { name: 'the installed tile layers', nativeMaxZoom: 4, maxZoom: 7, focusZoom: 6 },
    { name: 'the coordinate grid fallback', nativeMaxZoom: 6, maxZoom: 9, focusZoom: 8 },
  ])('extends $name past its last real tile level', ({ nativeMaxZoom, maxZoom, focusZoom }) => {
    expect(palworldMapMaxZoom(nativeMaxZoom)).toBe(maxZoom)
    expect(palworldMapFocusZoom(nativeMaxZoom)).toBe(focusZoom)
    expect(palworldMapFocusZoom(nativeMaxZoom)).toBeLessThan(palworldMapMaxZoom(nativeMaxZoom))
  })

  it.each([
    { zoom: 3, expected: false },
    { zoom: 4, expected: false },
    { zoom: 5, expected: true },
    { zoom: 7, expected: true },
  ])('draws icon markers at zoom $zoom: $expected', ({ zoom, expected }) => {
    expect(palworldMapUsesIconMarkers(zoom, 4)).toBe(expected)
  })

  it('grows the dot radius with zoom and clamps it at both ends', () => {
    expect(palworldMapDotRadius(0, 4)).toBe(palworldDotMetrics.minRadius)
    expect(palworldMapDotRadius(4, 4)).toBe(palworldDotMetrics.maxRadius)
    expect(palworldMapDotRadius(9, 4)).toBe(palworldDotMetrics.maxRadius)
    expect(palworldMapDotRadius(2, 4)).toBeGreaterThan(palworldMapDotRadius(1, 4))
  })

  it('adds a fixed bonus for active and selected dots on top of the zoom radius', () => {
    const base = palworldMapDotRadius(2, 4)

    expect(palworldMapDotRadius(2, 4, { active: true })).toBe(base + palworldDotMetrics.activeBonus)
    expect(palworldMapDotRadius(2, 4, { selected: true })).toBe(
      base + palworldDotMetrics.selectedBonus,
    )
    // Selection wins so the inspected actor never shrinks below an active peer.
    expect(palworldMapDotRadius(2, 4, { active: true, selected: true })).toBe(
      base + palworldDotMetrics.selectedBonus,
    )
  })

  it('grows the icon chip with every level past the tile source and caps it', () => {
    expect(palworldMapIconSize(5, 4)).toBeLessThan(palworldMapIconSize(6, 4))
    expect(palworldMapIconSize(6, 4)).toBeLessThan(palworldMapIconSize(7, 4))
    expect(palworldMapIconSize(7, 4)).toBe(palworldMapIconSize(12, 4))
  })

  // Shapes are reused across kinds on purpose: only three silhouettes exist, and
  // the kinds that share one are far apart in hue. What must hold is that no two
  // kinds are told apart by colour alone.
  it('never leaves two dot kinds sharing both a silhouette and a colour', () => {
    const dotKinds = palworldMapCategories.filter((category) => !category.labeledMarker)
    const signatures = dotKinds.map((category) => `${category.shape}:${category.colorToken}`)

    expect(new Set(signatures).size).toBe(dotKinds.length)
  })

  it('splits Companion Pals and NPCs, which used to share a near-identical cyan', () => {
    const companion = palworldMapCategory(PalworldMapActorKind.COMPANION_PAL)
    const npc = palworldMapCategory(PalworldMapActorKind.NPC)

    expect(npc.colorToken).not.toBe(companion.colorToken)
    expect(npc.shape).not.toBe(companion.shape)
    expect(palworldMapCategory(PalworldMapActorKind.WILD_PAL).shape).toBe('diamond')
  })
})
