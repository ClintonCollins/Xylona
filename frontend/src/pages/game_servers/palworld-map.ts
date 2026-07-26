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
  // A label merge replaces pills that would sit on top of each other, so it
  // draws at pill height rather than beneath them like a density cluster.
  labelMerge: boolean
  // Populated for label merges only. A density cluster can hold thousands of
  // actors and nothing needs their identities, but a merge has to be able to
  // hand one back when zooming can no longer separate the stack.
  actorKeys: readonly string[]
}

export interface PalworldMapRenderPlan {
  actors: PalworldMapActor[]
  clusters: PalworldMapCluster[]
  aggregatedActorCount: number
  // Labeled actors whose name would collide with a higher-priority label and so
  // render as an icon only.
  compactActorKeys: ReadonlySet<string>
  // Actors folded into a label merge. Fed back on the next render so a stack
  // needs more clearance to come apart than it needed to form.
  mergedActorKeys: ReadonlySet<string>
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
  previousMergedActorKeys?: ReadonlySet<string>
}

// Lower wins. Selection and search are explicit requests to see one actor, so
// they outrank everything; players outrank the rest of the world.
export const palworldLabelPriorities = {
  selected: 0,
  protected: 1,
  player: 2,
  ordinary: 3,
} as const

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
  // Only ordinary world actors merge. Bases are static and gain the most from
  // it, while a player folded into an anonymous chip is the opposite of what an
  // operational map is for; players collapse to an icon instead, and a selected
  // or searched-for actor keeps its full pill however tightly packed the spot.
  minMergePriority: palworldLabelPriorities.ordinary,
  // Merging and un-merging is a bigger visual jump than hiding a name, so it
  // needs more clearance to come apart than it did to form. Without this,
  // two Pals wandering around each other pop between pills and a chip on
  // every snapshot.
  mergeHysteresis: 10,
} as const

// The tile sources stop at their last downloaded level, so Leaflet upscales
// beyond it. Imagery softens but markers stay sharp, which is the point: these
// levels exist to pull crowded actors apart, not to reveal more terrain.
export const palworldZoomMetrics = {
  extraLevels: 3,
  focusOffset: 2,
  minIconSize: 19,
  iconSizeStep: 5,
  maxIconSize: 29,
} as const

export const palworldDotMetrics = {
  minRadius: 4,
  maxRadius: 6.5,
  activeBonus: 1,
  selectedBonus: 2.5,
  // Inactive dots used to sit at 0.4 and disappeared into the terrain.
  inactiveFillOpacity: 0.72,
  activeFillOpacity: 0.88,
  selectedFillOpacity: 1,
} as const

export function palworldMapMaxZoom(nativeMaxZoom: number): number {
  return nativeMaxZoom + palworldZoomMetrics.extraLevels
}

export function palworldMapFocusZoom(nativeMaxZoom: number): number {
  return nativeMaxZoom + palworldZoomMetrics.focusOffset
}

// Past the last real tile level the viewport holds few enough actors that DOM
// icons are affordable, and they read far better than a coloured dot.
export function palworldMapUsesIconMarkers(zoom: number, nativeMaxZoom: number): boolean {
  return zoom > nativeMaxZoom
}

export function palworldMapIconSize(zoom: number, nativeMaxZoom: number): number {
  const level = Math.min(
    palworldZoomMetrics.extraLevels,
    Math.max(1, Math.round(zoom) - nativeMaxZoom),
  )
  return Math.min(
    palworldZoomMetrics.maxIconSize,
    palworldZoomMetrics.minIconSize + (level - 1) * palworldZoomMetrics.iconSizeStep,
  )
}

export function palworldMapDotRadius(
  zoom: number,
  nativeMaxZoom: number,
  state: { active?: boolean; selected?: boolean } = {},
): number {
  const growth = Math.min(1, Math.max(0, zoom / Math.max(1, nativeMaxZoom)))
  const radius =
    palworldDotMetrics.minRadius +
    growth * (palworldDotMetrics.maxRadius - palworldDotMetrics.minRadius)
  if (state.selected === true) {
    return radius + palworldDotMetrics.selectedBonus
  }
  if (state.active === true) {
    return radius + palworldDotMetrics.activeBonus
  }
  return radius
}

export function palworldMapDotFillOpacity(state: { active?: boolean; selected?: boolean }): number {
  if (state.selected === true) {
    return palworldDotMetrics.selectedFillOpacity
  }
  return state.active === true
    ? palworldDotMetrics.activeFillOpacity
    : palworldDotMetrics.inactiveFillOpacity
}

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

// Colour alone failed at dot size: Companion Pals and NPCs sat a few degrees of
// hue apart, and every kind read as the same circle. Silhouette carries the
// distinction where colour cannot.
export type PalworldMapMarkerShape = 'circle' | 'diamond' | 'square'

export interface PalworldMapCategory {
  kind: PalworldMapActorKind
  label: string
  singular: string
  icon: string
  colorToken: string
  defaultVisible: boolean
  labeledMarker: boolean
  shape: PalworldMapMarkerShape
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
    shape: 'circle',
  },
  {
    kind: PalworldMapActorKind.BASE,
    label: 'Bases',
    singular: 'Base',
    icon: 'home_work',
    colorToken: '--xy-warning',
    defaultVisible: true,
    labeledMarker: true,
    shape: 'circle',
  },
  {
    kind: PalworldMapActorKind.BASE_WORKER,
    label: 'Base workers',
    singular: 'Base worker',
    icon: 'engineering',
    colorToken: '--xy-success',
    defaultVisible: true,
    labeledMarker: false,
    shape: 'circle',
  },
  {
    kind: PalworldMapActorKind.COMPANION_PAL,
    label: 'Companion Pals',
    singular: 'Companion Pal',
    icon: 'pets',
    colorToken: '--xy-accent',
    defaultVisible: true,
    labeledMarker: false,
    shape: 'circle',
  },
  {
    kind: PalworldMapActorKind.WILD_PAL,
    label: 'Wild Pals',
    singular: 'Wild Pal',
    icon: 'cruelty_free',
    colorToken: '--xy-purple',
    defaultVisible: true,
    labeledMarker: false,
    shape: 'diamond',
  },
  {
    kind: PalworldMapActorKind.NPC,
    label: 'NPCs',
    singular: 'NPC',
    icon: 'record_voice_over',
    colorToken: '--xy-category-7',
    defaultVisible: true,
    labeledMarker: false,
    shape: 'square',
  },
  {
    kind: PalworldMapActorKind.OTHER,
    label: 'Other actors',
    singular: 'Actor',
    icon: 'adjust',
    colorToken: '--xy-text-secondary',
    defaultVisible: true,
    labeledMarker: false,
    shape: 'square',
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
      shape: 'square',
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
    return palworldLabelPriorities.selected
  }
  // A search hit or a focused guild is an explicit request to see that actor, so
  // it outranks the category default.
  if (protectedActorKeys.has(actor.key)) {
    return palworldLabelPriorities.protected
  }
  return actor.kind === PalworldMapActorKind.PLAYER
    ? palworldLabelPriorities.player
    : palworldLabelPriorities.ordinary
}

interface PalworldLabelOptions {
  project: (actor: PalworldMapActor) => PalworldMapPoint
  selectedActorKey: string
  protectedActorKeys: ReadonlySet<string>
  previousCompactActorKeys: ReadonlySet<string>
  previousMergedActorKeys: ReadonlySet<string>
}

interface PalworldPlacedLabel {
  actor: PalworldMapActor
  priority: number
  labelRect: PalworldLabelRect
  iconRect: PalworldLabelRect
  merged: PalworldMapActor[]
}

interface PalworldLabelPlan {
  compactActorKeys: ReadonlySet<string>
  merges: PalworldMapCluster[]
  mergedActorKeys: ReadonlySet<string>
}

// Built fresh rather than shared: the returned sets escape into the render plan
// and on to the component, so a single stray mutation of a module-level
// singleton would poison every later render in a way that is near impossible to
// trace. This path runs at most once per render, so the allocation is free.
function emptyLabelPlan(): PalworldLabelPlan {
  return {
    compactActorKeys: new Set<string>(),
    merges: [],
    mergedActorKeys: new Set<string>(),
  }
}

function palworldMergeCluster(
  key: string,
  members: readonly PalworldMapActor[],
): PalworldMapCluster {
  let totalX = 0
  let totalY = 0
  let minX = Number.POSITIVE_INFINITY
  let minY = Number.POSITIVE_INFINITY
  let maxX = Number.NEGATIVE_INFINITY
  let maxY = Number.NEGATIVE_INFINITY
  for (const member of members) {
    totalX += member.locationX
    totalY += member.locationY
    minX = Math.min(minX, member.locationX)
    minY = Math.min(minY, member.locationY)
    maxX = Math.max(maxX, member.locationX)
    maxY = Math.max(maxY, member.locationY)
  }
  return {
    key,
    kind: members[0]?.kind ?? PalworldMapActorKind.OTHER,
    count: members.length,
    locationX: totalX / members.length,
    locationY: totalY / members.length,
    minX,
    minY,
    maxX,
    maxY,
    labelMerge: true,
    actorKeys: members.map((member) => member.key),
  }
}

// The pairwise scan is quadratic, so a crowded viewport falls back to screen-cell
// bucketing. It reaches the same three rungs in one pass rather than compacting
// every pill wholesale and leaving them stacked.
function resolvePalworldBucketedLabelPlan(
  candidates: readonly PalworldMapActor[],
  options: PalworldLabelOptions,
): PalworldLabelPlan {
  const compactActorKeys = new Set<string>()
  const mergedActorKeys = new Set<string>()
  const buckets = new Map<string, PalworldMapActor[]>()
  for (const actor of candidates) {
    const priority = palworldLabelPriority(
      actor,
      options.selectedActorKey,
      options.protectedActorKeys,
    )
    if (priority < palworldLabelMetrics.minMergePriority) {
      continue
    }
    const point = options.project(actor)
    const cellX = Math.floor(point.x / palworldLabelMetrics.pillHeight)
    const cellY = Math.floor(point.y / palworldLabelMetrics.pillHeight)
    const bucketKey = `${actor.kind}:${cellX}:${cellY}`
    const bucket = buckets.get(bucketKey)
    if (bucket === undefined) {
      buckets.set(bucketKey, [actor])
      continue
    }
    bucket.push(actor)
  }

  const merges: PalworldMapCluster[] = []
  for (const [bucketKey, members] of [...buckets.entries()].sort(([left], [right]) =>
    compareMapKeys(left, right),
  )) {
    if (members.length < 2) {
      const only = members[0]
      if (
        only !== undefined &&
        palworldLabelPriority(only, options.selectedActorKey, options.protectedActorKeys) >=
          palworldLabelPriorities.ordinary
      ) {
        compactActorKeys.add(only.key)
      }
      continue
    }
    const ordered = members.toSorted((left, right) => compareMapKeys(left.key, right.key))
    merges.push(palworldMergeCluster(`merge:${bucketKey}`, ordered))
    for (const member of ordered) {
      mergedActorKeys.add(member.key)
    }
  }
  return { compactActorKeys, merges, mergedActorKeys }
}

function resolvePalworldLabelPlan(
  actors: readonly PalworldMapActor[],
  options: PalworldLabelOptions,
): PalworldLabelPlan {
  const candidates = actors.filter((actor) => palworldMapCategory(actor.kind).labeledMarker)
  if (candidates.length < 2) {
    return emptyLabelPlan()
  }
  if (candidates.length > palworldLabelMetrics.maxLabelCandidates) {
    return resolvePalworldBucketedLabelPlan(candidates, options)
  }

  const ranked = candidates
    .map((actor) => ({
      actor,
      priority: palworldLabelPriority(actor, options.selectedActorKey, options.protectedActorKeys),
    }))
    .toSorted((left, right) => {
      if (left.priority !== right.priority) {
        return left.priority - right.priority
      }
      return compareMapKeys(left.actor.key, right.actor.key)
    })

  const compactActorKeys = new Set<string>()
  const placed: PalworldPlacedLabel[] = []
  for (const { actor, priority } of ranked) {
    const point = options.project(actor)
    const iconRect = palworldLabelRect(
      point,
      palworldLabelMetrics.pillHeight,
      palworldLabelMetrics.collisionPadding +
        (options.previousMergedActorKeys.has(actor.key) ? palworldLabelMetrics.mergeHysteresis : 0),
    )
    // Merging is decided on icon boxes alone: two pills whose names collide can
    // still both be read once collapsed, but two overlapping icons cannot.
    // Kinds never mix, so the chip's count always describes one thing.
    if (priority >= palworldLabelMetrics.minMergePriority) {
      const anchor = placed.find(
        (existing) =>
          existing.actor.kind === actor.kind &&
          existing.priority >= palworldLabelMetrics.minMergePriority &&
          palworldLabelRectsOverlap(iconRect, existing.iconRect),
      )
      if (anchor !== undefined) {
        anchor.merged.push(actor)
        continue
      }
    }
    const plainIconRect = palworldLabelRect(point, palworldLabelMetrics.pillHeight, 0)
    const fullWidth =
      palworldLabelMetrics.iconSectionWidth +
      estimatePalworldLabelWidth(actor.name) +
      palworldLabelMetrics.rightPadding
    const inflate =
      palworldLabelMetrics.collisionPadding +
      (options.previousCompactActorKeys.has(actor.key) ? palworldLabelMetrics.expandHysteresis : 0)
    // Hoisted out of the callback: this ran once per already-placed label, so a
    // crowded viewport allocated tens of thousands of throwaway rects per frame.
    const candidateLabelRect = palworldLabelRect(point, fullWidth, inflate)
    const collides = placed.some((existing) =>
      palworldLabelRectsOverlap(candidateLabelRect, existing.labelRect),
    )
    if (collides && actor.key !== options.selectedActorKey) {
      compactActorKeys.add(actor.key)
      placed.push({
        actor,
        priority,
        labelRect: plainIconRect,
        iconRect: plainIconRect,
        merged: [],
      })
      continue
    }
    placed.push({
      actor,
      priority,
      labelRect: palworldLabelRect(point, fullWidth, 0),
      iconRect: plainIconRect,
      merged: [],
    })
  }

  const merges: PalworldMapCluster[] = []
  const mergedActorKeys = new Set<string>()
  for (const entry of placed) {
    if (entry.merged.length === 0) {
      continue
    }
    const members = [entry.actor, ...entry.merged].toSorted((left, right) =>
      compareMapKeys(left.key, right.key),
    )
    merges.push(palworldMergeCluster(`merge:${entry.actor.key}`, members))
    for (const member of members) {
      mergedActorKeys.add(member.key)
      compactActorKeys.delete(member.key)
    }
  }
  return { compactActorKeys, merges, mergedActorKeys }
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
    previousMergedActorKeys: options.previousMergedActorKeys ?? new Set<string>(),
  }
  // Only the returned collections need a deterministic order; bucketing below is
  // order-independent because centroids are means and buckets are sorted by key.
  const visibleActors = sourceActors.filter((actor) => actorWithinBounds(actor, options.bounds))

  if (options.zoom >= clusterBelowZoom && visibleActors.length <= maxIndividualActors) {
    const planActors = visibleActors.toSorted((left, right) => compareMapKeys(left.key, right.key))
    return finalizePalworldMapPlan(planActors, [], 0, labelOptions)
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
      labelMerge: false,
      actorKeys: [],
    })
  }

  const planActors = actors.toSorted((left, right) => compareMapKeys(left.key, right.key))
  return finalizePalworldMapPlan(planActors, clusters, aggregatedActorCount, labelOptions)
}

// Label merges are density groups too, so they ride the same cluster channel and
// count toward the "grouped at this zoom" telemetry.
function finalizePalworldMapPlan(
  planActors: readonly PalworldMapActor[],
  clusters: readonly PalworldMapCluster[],
  aggregatedActorCount: number,
  labelOptions: PalworldLabelOptions,
): PalworldMapRenderPlan {
  const labelPlan = resolvePalworldLabelPlan(planActors, labelOptions)
  if (labelPlan.merges.length === 0) {
    return {
      actors: [...planActors],
      clusters: [...clusters],
      aggregatedActorCount,
      compactActorKeys: labelPlan.compactActorKeys,
      mergedActorKeys: labelPlan.mergedActorKeys,
    }
  }
  const mergedActorCount = labelPlan.merges.reduce((total, merge) => total + merge.count, 0)
  return {
    actors: planActors.filter((actor) => !labelPlan.mergedActorKeys.has(actor.key)),
    clusters: [...clusters, ...labelPlan.merges],
    aggregatedActorCount: aggregatedActorCount + mergedActorCount,
    compactActorKeys: labelPlan.compactActorKeys,
    mergedActorKeys: labelPlan.mergedActorKeys,
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
