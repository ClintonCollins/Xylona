import { PalworldMapActorKind, type PalworldMapActor } from '@/proto/xylona_pb'

export interface PalworldMapPoint {
  x: number
  y: number
}

export interface PalworldMapBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

export interface PalworldMapCluster {
  key: string
  kind: PalworldMapActorKind
  count: number
  locationX: number
  locationY: number
  minX: number
  minY: number
  maxX: number
  maxY: number
}

export interface PalworldMapRenderPlan {
  actors: PalworldMapActor[]
  clusters: PalworldMapCluster[]
  aggregatedActorCount: number
}

export interface PalworldMapRenderOptions {
  zoom: number
  clusterBelowZoom?: number
  clusterCellSize?: number
  maxIndividualActors?: number
  bounds?: PalworldMapBounds
  protectedActorKeys?: ReadonlySet<string>
  project?: (actor: PalworldMapActor) => PalworldMapPoint
}

export interface PalworldMapCategory {
  kind: PalworldMapActorKind
  label: string
  singular: string
  icon: string
  colorToken: string
  defaultVisible: boolean
  labeledMarker: boolean
}

export const palworldMapCategories: PalworldMapCategory[] = [
  {
    kind: PalworldMapActorKind.PLAYER,
    label: 'Players',
    singular: 'Player',
    icon: 'person',
    colorToken: '--xy-primary',
    defaultVisible: true,
    labeledMarker: true,
  },
  {
    kind: PalworldMapActorKind.BASE,
    label: 'Bases',
    singular: 'Base',
    icon: 'home_work',
    colorToken: '--xy-warning',
    defaultVisible: true,
    labeledMarker: true,
  },
  {
    kind: PalworldMapActorKind.BASE_WORKER,
    label: 'Base workers',
    singular: 'Base worker',
    icon: 'engineering',
    colorToken: '--xy-success',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.COMPANION_PAL,
    label: 'Companion Pals',
    singular: 'Companion Pal',
    icon: 'pets',
    colorToken: '--xy-accent',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.WILD_PAL,
    label: 'Wild Pals',
    singular: 'Wild Pal',
    icon: 'cruelty_free',
    colorToken: '--xy-purple',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.NPC,
    label: 'NPCs',
    singular: 'NPC',
    icon: 'record_voice_over',
    colorToken: '--xy-info',
    defaultVisible: true,
    labeledMarker: false,
  },
  {
    kind: PalworldMapActorKind.OTHER,
    label: 'Other actors',
    singular: 'Actor',
    icon: 'adjust',
    colorToken: '--xy-text-secondary',
    defaultVisible: true,
    labeledMarker: false,
  },
]

const categoriesByKind = new Map(palworldMapCategories.map((category) => [category.kind, category]))

export function palworldMapCategory(kind: PalworldMapActorKind): PalworldMapCategory {
  return (
    categoriesByKind.get(kind) ?? {
      kind,
      label: 'Other actors',
      singular: 'Actor',
      icon: 'adjust',
      colorToken: '--xy-text-secondary',
      defaultVisible: false,
      labeledMarker: false,
    }
  )
}

export function initialPalworldMapVisibility(): Record<number, boolean> {
  return Object.fromEntries(
    palworldMapCategories.map((category) => [category.kind, category.defaultVisible]),
  )
}

export function filterPalworldMapActors(
  actors: PalworldMapActor[],
  visibleKinds: Record<number, boolean>,
  search: string,
): PalworldMapActor[] {
  const normalizedSearch = search.trim().toLocaleLowerCase()
  return actors.filter((actor) => {
    if (!visibleKinds[actor.kind]) {
      return false
    }
    if (normalizedSearch === '') {
      return true
    }
    return [actor.name, actor.guildName, actor.trainerName, actor.className].some((value) =>
      value.toLocaleLowerCase().includes(normalizedSearch),
    )
  })
}

export function formatPalworldCoordinate(value: number): string {
  return Number.isFinite(value) ? value.toFixed(2) : '—'
}

export function palworldMapGuildKey(actor: PalworldMapActor): string {
  const guildKey = actor.guildKey.trim()
  if (guildKey !== '') {
    return `key:${guildKey}`
  }
  const guildName = actor.guildName.trim().toLocaleLowerCase()
  return guildName === '' ? '' : `name:${guildName}`
}

export function formatPalworldFacing(rotationZ: number): string {
  if (!Number.isFinite(rotationZ)) {
    return '—'
  }
  const normalized = ((rotationZ % 360) + 360) % 360
  const directions = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'] as const
  const direction = directions[Math.round(normalized / 45) % directions.length]
  return `${direction} ${normalized.toFixed(0)}°`
}

export function formatPalworldUptime(seconds: bigint | number): string {
  const numericSeconds = typeof seconds === 'bigint' ? Number(seconds) : seconds
  if (!Number.isFinite(numericSeconds) || numericSeconds < 0) {
    return '—'
  }
  const totalMinutes = Math.floor(numericSeconds / 60)
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) {
    return `${days}d ${hours}h`
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  return `${minutes}m`
}

export function buildPalworldMapRenderPlan(
  sourceActors: readonly PalworldMapActor[],
  options: PalworldMapRenderOptions,
): PalworldMapRenderPlan {
  const clusterBelowZoom = options.clusterBelowZoom ?? 4
  const clusterCellSize = Math.max(24, options.clusterCellSize ?? 72)
  const maxIndividualActors = Math.max(1, options.maxIndividualActors ?? 1_500)
  const protectedActorKeys = options.protectedActorKeys ?? new Set<string>()
  const project =
    options.project ??
    ((actor: PalworldMapActor): PalworldMapPoint => ({
      x: actor.locationX,
      y: actor.locationY,
    }))
  const visibleActors = sourceActors
    .filter((actor) => actorWithinBounds(actor, options.bounds))
    .toSorted((left, right) => left.key.localeCompare(right.key))

  if (options.zoom >= clusterBelowZoom && visibleActors.length <= maxIndividualActors) {
    return {
      actors: visibleActors,
      clusters: [],
      aggregatedActorCount: 0,
    }
  }

  const actors: PalworldMapActor[] = []
  const buckets = new Map<
    string,
    {
      kind: PalworldMapActorKind
      actors: PalworldMapActor[]
      totalX: number
      totalY: number
      minX: number
      minY: number
      maxX: number
      maxY: number
    }
  >()

  for (const actor of visibleActors) {
    if (
      actor.kind === PalworldMapActorKind.PLAYER ||
      actor.kind === PalworldMapActorKind.BASE ||
      protectedActorKeys.has(actor.key)
    ) {
      actors.push(actor)
      continue
    }

    const point = project(actor)
    const cellX = Math.floor(point.x / clusterCellSize)
    const cellY = Math.floor(point.y / clusterCellSize)
    const bucketKey = `${actor.kind}:${cellX}:${cellY}`
    const bucket = buckets.get(bucketKey)
    if (bucket === undefined) {
      buckets.set(bucketKey, {
        kind: actor.kind,
        actors: [actor],
        totalX: actor.locationX,
        totalY: actor.locationY,
        minX: actor.locationX,
        minY: actor.locationY,
        maxX: actor.locationX,
        maxY: actor.locationY,
      })
      continue
    }
    bucket.actors.push(actor)
    bucket.totalX += actor.locationX
    bucket.totalY += actor.locationY
    bucket.minX = Math.min(bucket.minX, actor.locationX)
    bucket.minY = Math.min(bucket.minY, actor.locationY)
    bucket.maxX = Math.max(bucket.maxX, actor.locationX)
    bucket.maxY = Math.max(bucket.maxY, actor.locationY)
  }

  const clusters: PalworldMapCluster[] = []
  let aggregatedActorCount = 0
  for (const [bucketKey, bucket] of [...buckets.entries()].sort(([left], [right]) =>
    left.localeCompare(right),
  )) {
    if (bucket.actors.length === 1) {
      const onlyActor = bucket.actors[0]
      if (onlyActor !== undefined) {
        actors.push(onlyActor)
      }
      continue
    }
    aggregatedActorCount += bucket.actors.length
    clusters.push({
      key: `cluster:${bucketKey}`,
      kind: bucket.kind,
      count: bucket.actors.length,
      locationX: bucket.totalX / bucket.actors.length,
      locationY: bucket.totalY / bucket.actors.length,
      minX: bucket.minX,
      minY: bucket.minY,
      maxX: bucket.maxX,
      maxY: bucket.maxY,
    })
  }

  return {
    actors: actors.toSorted((left, right) => left.key.localeCompare(right.key)),
    clusters,
    aggregatedActorCount,
  }
}

function actorWithinBounds(
  actor: PalworldMapActor,
  bounds: PalworldMapBounds | undefined,
): boolean {
  if (bounds === undefined) {
    return true
  }
  return (
    actor.locationX >= bounds.minX &&
    actor.locationX <= bounds.maxX &&
    actor.locationY >= bounds.minY &&
    actor.locationY <= bounds.maxY
  )
}
