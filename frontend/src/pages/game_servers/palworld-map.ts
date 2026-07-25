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
  // Labeled actors whose name would collide with a higher-priority label and so
  // render as an icon only.
  compactActorKeys: ReadonlySet<string>
}

export interface PalworldMapRenderOptions {
  zoom: number
  clusterBelowZoom?: number
  clusterCellSize?: number
  maxIndividualActors?: number
  bounds?: PalworldMapBounds
  protectedActorKeys?: ReadonlySet<string>
  project?: (actor: PalworldMapActor) => PalworldMapPoint
  selectedActorKey?: string
  previousCompactActorKeys?: ReadonlySet<string>
}

// Mirrors the .palworld-map-marker CSS box. Label width is estimated rather than
// measured because this module never touches the DOM; the estimate only has to
// be close, since the label is ellipsised at maxLabelWidth either way.
export const palworldLabelMetrics = {
  anchor: 17,
  pillHeight: 34,
  iconSectionWidth: 34,
  rightPadding: 9,
  avgCharWidth: 7,
  maxLabelWidth: 180,
  collisionPadding: 4,
  // A hidden label must clear its neighbour by more than it needed to be hidden,
  // otherwise labels flicker on and off while the map is panned.
  expandHysteresis: 8,
  maxLabelCandidates: 400,
} as const

interface PalworldLabelRect {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

export function estimatePalworldLabelWidth(name: string): number {
  return Math.min(
    palworldLabelMetrics.maxLabelWidth,
    Math.max(1, name.length) * palworldLabelMetrics.avgCharWidth,
  )
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

// compareMapKeys orders by UTF-16 code unit instead of Intl collation. The
// keys are opaque hashes and bucket coordinates, so collation orders them by
// the viewer's locale for no benefit; ordinal comparison is both stable across
// locales and measurably cheaper on the pan and zoom render path.
function compareMapKeys(left: string, right: string): number {
  if (left < right) {
    return -1
  }
  if (left > right) {
    return 1
  }
  return 0
}

export function assignPalworldBaseWorkers(actors: readonly PalworldMapActor[]): {
  byBase: ReadonlyMap<string, readonly PalworldMapActor[]>
  unassigned: readonly PalworldMapActor[]
} {
  const basesByGuild = new Map<string, PalworldMapActor[]>()
  const byBase = new Map<string, PalworldMapActor[]>()
  for (const actor of actors) {
    if (actor.kind !== PalworldMapActorKind.BASE) {
      continue
    }
    byBase.set(actor.key, [])
    const guildKey = palworldMapGuildKey(actor)
    if (guildKey === '') {
      continue
    }
    const guildBases = basesByGuild.get(guildKey)
    if (guildBases === undefined) {
      basesByGuild.set(guildKey, [actor])
      continue
    }
    guildBases.push(actor)
  }

  const unassigned: PalworldMapActor[] = []
  for (const actor of actors) {
    if (actor.kind !== PalworldMapActorKind.BASE_WORKER) {
      continue
    }
    const guildBases = basesByGuild.get(palworldMapGuildKey(actor)) ?? []
    let nearestBase: PalworldMapActor | null = null
    let nearestDistance = Number.POSITIVE_INFINITY
    for (const candidate of guildBases) {
      const distance =
        (candidate.locationX - actor.locationX) ** 2 + (candidate.locationY - actor.locationY) ** 2
      if (distance < nearestDistance) {
        nearestBase = candidate
        nearestDistance = distance
      }
    }
    if (nearestBase === null) {
      unassigned.push(actor)
      continue
    }
    byBase.get(nearestBase.key)?.push(actor)
  }

  for (const workers of byBase.values()) {
    workers.sort((left, right) => compareMapKeys(left.key, right.key))
  }
  unassigned.sort((left, right) => compareMapKeys(left.key, right.key))
  return { byBase, unassigned }
}

function palworldLabelRect(
  point: PalworldMapPoint,
  width: number,
  inflate: number,
): PalworldLabelRect {
  const left = point.x - palworldLabelMetrics.anchor
  const top = point.y - palworldLabelMetrics.anchor
  return {
    minX: left - inflate,
    minY: top - inflate,
    maxX: left + width + inflate,
    maxY: top + palworldLabelMetrics.pillHeight + inflate,
  }
}

function palworldLabelRectsOverlap(left: PalworldLabelRect, right: PalworldLabelRect): boolean {
  return (
    left.minX < right.maxX &&
    right.minX < left.maxX &&
    left.minY < right.maxY &&
    right.minY < left.maxY
  )
}

function palworldLabelPriority(
  actor: PalworldMapActor,
  selectedActorKey: string,
  protectedActorKeys: ReadonlySet<string>,
): number {
  if (actor.key === selectedActorKey) {
    return 0
  }
  // A search hit or a focused guild is an explicit request to see that actor, so
  // it outranks the category default.
  if (protectedActorKeys.has(actor.key)) {
    return 1
  }
  return actor.kind === PalworldMapActorKind.PLAYER ? 2 : 3
}

function resolvePalworldCompactLabelKeys(
  actors: readonly PalworldMapActor[],
  options: {
    project: (actor: PalworldMapActor) => PalworldMapPoint
    selectedActorKey: string
    protectedActorKeys: ReadonlySet<string>
    previousCompactActorKeys: ReadonlySet<string>
  },
): ReadonlySet<string> {
  const compactActorKeys = new Set<string>()
  const candidates = actors.filter((actor) => palworldMapCategory(actor.kind).labeledMarker)
  if (candidates.length < 2) {
    return compactActorKeys
  }
  if (candidates.length > palworldLabelMetrics.maxLabelCandidates) {
    for (const actor of candidates) {
      if (palworldLabelPriority(actor, options.selectedActorKey, options.protectedActorKeys) < 3) {
        continue
      }
      compactActorKeys.add(actor.key)
    }
    return compactActorKeys
  }

  const ranked = candidates.toSorted((left, right) => {
    const leftPriority = palworldLabelPriority(
      left,
      options.selectedActorKey,
      options.protectedActorKeys,
    )
    const rightPriority = palworldLabelPriority(
      right,
      options.selectedActorKey,
      options.protectedActorKeys,
    )
    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority
    }
    return compareMapKeys(left.key, right.key)
  })

  const placed: PalworldLabelRect[] = []
  for (const actor of ranked) {
    const point = options.project(actor)
    const fullWidth =
      palworldLabelMetrics.iconSectionWidth +
      estimatePalworldLabelWidth(actor.name) +
      palworldLabelMetrics.rightPadding
    const inflate =
      palworldLabelMetrics.collisionPadding +
      (options.previousCompactActorKeys.has(actor.key) ? palworldLabelMetrics.expandHysteresis : 0)
    const collides = placed.some((existing) =>
      palworldLabelRectsOverlap(palworldLabelRect(point, fullWidth, inflate), existing),
    )
    if (collides && actor.key !== options.selectedActorKey) {
      compactActorKeys.add(actor.key)
      placed.push(palworldLabelRect(point, palworldLabelMetrics.pillHeight, 0))
      continue
    }
    placed.push(palworldLabelRect(point, fullWidth, 0))
  }
  return compactActorKeys
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
  const labelOptions = {
    project,
    selectedActorKey: options.selectedActorKey ?? '',
    protectedActorKeys,
    previousCompactActorKeys: options.previousCompactActorKeys ?? new Set<string>(),
  }
  // Only the returned collections need a deterministic order; bucketing below is
  // order-independent because centroids are means and buckets are sorted by key.
  const visibleActors = sourceActors.filter((actor) => actorWithinBounds(actor, options.bounds))

  if (options.zoom >= clusterBelowZoom && visibleActors.length <= maxIndividualActors) {
    const planActors = visibleActors.toSorted((left, right) => compareMapKeys(left.key, right.key))
    return {
      actors: planActors,
      clusters: [],
      aggregatedActorCount: 0,
      compactActorKeys: resolvePalworldCompactLabelKeys(planActors, labelOptions),
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
    compareMapKeys(left, right),
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

  const planActors = actors.toSorted((left, right) => compareMapKeys(left.key, right.key))
  return {
    actors: planActors,
    clusters,
    aggregatedActorCount,
    compactActorKeys: resolvePalworldCompactLabelKeys(planActors, labelOptions),
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
