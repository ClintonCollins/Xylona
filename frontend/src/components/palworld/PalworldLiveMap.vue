<!--
THESIS: Turn the world map from an actor browser into an operational radar; refuse one-marker-per-actor noise at world scale.
OWN-WORLD: Xylona’s layered dark command surface, cyan/blue state, semantic category colors, compact mono telemetry.
STORY: Admin scans health, narrows population, inspects a guild or base, then focuses exact actors.
FIRST VIEWPORT: Map remains dominant; status toolbar above, compact intelligence chips and health telemetry at edges, contextual rail on demand.
FORM: Existing Operate surface extension; semantic zoom and progressive disclosure, no new visual world.
-->
<script lang="ts" setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import L, { type CircleMarker, type Map as LeafletMap, type Marker } from 'leaflet'
import 'leaflet/dist/leaflet.css'

import {
  PalworldMapActorKind,
  type PalworldMapActor,
  type PalworldMapLayer,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import {
  assignPalworldBaseWorkers,
  buildPalworldMapRenderPlan,
  filterPalworldMapActors,
  formatPalworldFacing,
  formatPalworldCoordinate,
  formatPalworldUptime,
  initialPalworldMapVisibility,
  palworldMapCategories,
  palworldMapCategory,
  palworldMapDotFillOpacity,
  palworldMapDotRadius,
  palworldMapFocusZoom,
  palworldMapGuildKey,
  palworldMapIconSize,
  palworldMapMaxZoom,
  palworldMapUsesIconMarkers,
  type PalworldMapCluster,
} from '@/pages/game_servers/palworld-map'
import { palworldShapeMarker } from './palworld-shape-marker'

const props = withDefaults(
  defineProps<{
    view: PalworldMapView | null
    loading?: boolean
    loadError?: boolean
    publicMode?: boolean
  }>(),
  {
    loading: false,
    loadError: false,
    publicMode: false,
  },
)

const emit = defineEmits<{
  refresh: []
}>()

const fallbackLayer: PalworldMapLayer = {
  $typeName: 'xylona.PalworldMapLayer',
  id: 'coordinate-grid',
  label: 'Coordinate grid',
  tileUrlTemplate: '',
  attribution: '',
  minZoom: 0,
  maxZoom: 6,
  tileSize: 512,
  transformA: 2048 / 1_800_000,
  transformB: 1024,
  transformC: -2048 / 1_950_000,
  transformD: 2048 * (750_000 / 1_950_000),
  minX: -1_200_000,
  minY: -900_000,
  maxX: 750_000,
  maxY: 900_000,
}

const mapRoot = ref<HTMLElement | null>(null)
const mapElement = ref<HTMLElement | null>(null)
const railOpen = ref(false)
const moreKindsOpen = ref(false)
const fullscreen = ref(false)
const search = ref('')
const visibleKinds = ref<Record<number, boolean>>(initialPalworldMapVisibility())
const activeLayerID = ref('')
const selectedActorKey = ref('')
const focusedKind = ref<PalworldMapActorKind | null>(null)
const focusedGuildKey = ref('')
const aggregatedActorCount = ref(0)

let map: LeafletMap | null = null
let actorLayer: L.LayerGroup | null = null
let resizeObserver: ResizeObserver | null = null
let renderFrame = 0
let currentLayerKey = ''
let lastSnapshotKey = ''
let fittedOnce = false
let railInitialized = false
const actorMarkers = new Map<string, Marker | CircleMarker>()
const actorMarkerRenderKeys = new Map<string, string>()
const actorMarkerStateKeys = new Map<string, string>()
// A focus request outlives the marker it started on: zooming past the tile
// source rebuilds that actor as an icon marker, so the tooltip is opened by
// whichever render leaves the view settled.
let pendingFocus: { actorKey: string; zoom: number } | null = null
const clusterMarkers = new Map<string, Marker>()
const clusterMarkerRenderKeys = new Map<string, string>()
const maxIndividualActors = 1_500
let compactActorKeys: ReadonlySet<string> = new Set<string>()
let mergedActorKeys: ReadonlySet<string> = new Set<string>()

const actors = computed(() => {
  const reportedActors = props.view?.actors ?? []
  if (!props.view?.partial) {
    return reportedActors
  }
  return reportedActors.filter((actor) => actor.kind === PalworldMapActorKind.PLAYER)
})
const actorsByKey = computed(
  () => new Map(actors.value.map((actor) => [actor.key, actor] as const)),
)
const availableActorCategories = computed(() => {
  if (!props.view?.available) {
    return []
  }
  return props.view.partial
    ? palworldMapCategories.filter((category) => category.kind === PalworldMapActorKind.PLAYER)
    : palworldMapCategories
})
const primaryActorCategories = computed(() => availableActorCategories.value.slice(0, 4))
const secondaryActorCategories = computed(() => availableActorCategories.value.slice(4))
const actorPanelLabel = computed(() => (props.view?.partial ? 'players' : 'world actors'))
const actorPanelTitle = computed(() => (props.view?.partial ? 'Players' : 'World actors'))
const configuredLayers = computed(() => props.view?.layers ?? [])
const activeLayer = computed(() => {
  const configured = configuredLayers.value.find((layer) => layer.id === activeLayerID.value)
  return configured ?? configuredLayers.value[0] ?? fallbackLayer
})
const hasImagery = computed(() => activeLayer.value.tileUrlTemplate !== '')
const filteredActors = computed(() => {
  const matchingActors = filterPalworldMapActors(actors.value, visibleKinds.value, search.value)
  if (focusedGuildKey.value === '') {
    return matchingActors
  }
  return matchingActors.filter((actor) => palworldMapGuildKey(actor) === focusedGuildKey.value)
})
const selectedActor = computed(() => actorsByKey.value.get(selectedActorKey.value) ?? null)
// Base workers sit on top of their base, so drawing one marker each buried the
// base name under a stack of dots. The count rides on the base pill instead and
// the roster still lists every worker.
const baseWorkerAssignment = computed(() => assignPalworldBaseWorkers(actors.value))
const mappedActors = computed(() => {
  if (!visibleKinds.value[PalworldMapActorKind.BASE_WORKER]) {
    return filteredActors.value
  }
  const unassigned = new Set(baseWorkerAssignment.value.unassigned.map((actor) => actor.key))
  return filteredActors.value.filter(
    (actor) =>
      actor.kind !== PalworldMapActorKind.BASE_WORKER ||
      unassigned.has(actor.key) ||
      actor.key === selectedActorKey.value,
  )
})
const selectedBaseWorkers = computed(() => {
  const base = selectedActor.value
  if (base === null || base.kind !== PalworldMapActorKind.BASE) {
    return []
  }
  return baseWorkerAssignment.value.byBase.get(base.key) ?? []
})
const health = computed(() => props.view?.health ?? null)
const actorCounts = computed(() => {
  const counts = new Map<PalworldMapActorKind, number>()
  for (const actor of actors.value) {
    counts.set(actor.kind, (counts.get(actor.kind) ?? 0) + 1)
  }
  return counts
})
const secondaryActorCount = computed(() =>
  secondaryActorCategories.value.reduce(
    (count, category) => count + (actorCounts.value.get(category.kind) ?? 0),
    0,
  ),
)
const summaryCategories = computed(() =>
  availableActorCategories.value.filter((category) => category.kind !== PalworldMapActorKind.OTHER),
)
const countQualifier = computed(() => (props.view?.truncated ? 'At least ' : ''))
const selectedGuildActors = computed(() => {
  const actor = selectedActor.value
  if (actor === null || actor.kind !== PalworldMapActorKind.BASE) {
    return []
  }
  const guildKey = palworldMapGuildKey(actor)
  if (guildKey === '') {
    return actors.value.filter((candidate) => candidate.key === actor.key)
  }
  return actors.value.filter((candidate) => palworldMapGuildKey(candidate) === guildKey)
})
const selectedGuildCounts = computed(() => {
  const counts = new Map<PalworldMapActorKind, number>()
  for (const actor of selectedGuildActors.value) {
    counts.set(actor.kind, (counts.get(actor.kind) ?? 0) + 1)
  }
  return counts
})
const selectedGuildUsesNameEstimate = computed(() => {
  const actor = selectedActor.value
  return (
    actor !== null &&
    actor.kind === PalworldMapActorKind.BASE &&
    actor.guildKey.trim() === '' &&
    actor.guildName.trim() !== ''
  )
})
const selectedGuildWorkerCondition = computed(() => {
  let active = 0
  let injured = 0
  for (const actor of selectedGuildActors.value) {
    if (actor.kind !== PalworldMapActorKind.BASE_WORKER) {
      continue
    }
    if (actor.active) {
      active += 1
    }
    if (actor.maxHp > 0 && actor.hp < actor.maxHp) {
      injured += 1
    }
  }
  return { active, injured }
})
const mapStatus = computed(() => {
  if (props.loadError) {
    return { label: 'Connection lost', icon: 'cloud_off', tone: 'danger' }
  }
  if (props.loading && props.view === null) {
    return { label: 'Loading world', icon: 'sync', tone: 'info' }
  }
  if (!props.view?.available) {
    return {
      label: props.view?.serverOnline ? 'Waiting for data' : 'Server offline',
      icon: 'schedule',
      tone: 'warning',
    }
  }
  if (props.view.stale) {
    return { label: 'Last known positions', icon: 'history', tone: 'warning' }
  }
  if (props.view.partial) {
    return { label: 'Players only', icon: 'group', tone: 'info' }
  }
  return { label: 'Live world', icon: 'sensors', tone: 'success' }
})
// The map used to drop silently to player positions with no explanation, which
// reads as "bases are broken" rather than "the server is not reporting them".
const partialNotice = computed(() => {
  if (props.view?.available !== true || !props.view.partial) {
    return ''
  }
  if (props.view.partialReason !== '') {
    return props.view.partialReason
  }
  return 'This server is reporting player positions only, so bases, Pals, and NPCs are unavailable.'
})
const collectedLabel = computed(() => {
  const collectedAt = props.view?.collectedAt
  if (collectedAt === undefined) {
    return 'No snapshot received yet'
  }
  const ageSeconds = Math.max(
    0,
    Math.floor((Date.now() - timestampDate(collectedAt).getTime()) / 1_000),
  )
  if (ageSeconds < 5) {
    return 'Updated just now'
  }
  if (ageSeconds < 60) {
    return `Updated ${ageSeconds}s ago`
  }
  if (ageSeconds < 3_600) {
    return `Updated ${Math.floor(ageSeconds / 60)}m ago`
  }
  if (ageSeconds < 86_400) {
    return `Updated ${Math.floor(ageSeconds / 3_600)}h ago`
  }
  return `Updated ${Math.floor(ageSeconds / 86_400)}d ago`
})
const collectedTitle = computed(() => {
  const collectedAt = props.view?.collectedAt
  if (collectedAt === undefined) {
    return ''
  }
  return timestampDate(collectedAt).toLocaleString()
})

// Resolving a token forces a style recalculation, and markers resolve one every
// time they are built. The palette is static for the lifetime of the document,
// so the seven category colors are read once each.
const cssColorCache = new Map<string, string>()

function cssColor(token: string): string {
  const cached = cssColorCache.get(token)
  if (cached !== undefined) {
    return cached
  }
  const rootStyle = getComputedStyle(document.documentElement)
  const value = rootStyle.getPropertyValue(token).trim() || rootStyle.color
  cssColorCache.set(token, value)
  return value
}

function escapeHTML(value: string): string {
  return value.replace(
    /[&<>'"]/g,
    (character) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' })[character] ??
      character,
  )
}

function nativeMaxZoom(): number {
  return activeLayer.value.maxZoom
}

function currentZoom(): number {
  return map?.getZoom() ?? activeLayer.value.minZoom
}

function buildCRS(layer: PalworldMapLayer): L.CRS {
  return Object.assign({}, L.CRS.Simple, {
    transformation: new L.Transformation(
      layer.transformA,
      layer.transformB,
      layer.transformC,
      layer.transformD,
    ),
  }) as L.CRS
}

function worldLocation(locationX: number, locationY: number): L.LatLng {
  return L.latLng(locationX, locationY)
}

function layerBounds(layer: PalworldMapLayer): L.LatLngBounds {
  return L.latLngBounds(
    worldLocation(layer.minX, layer.minY),
    worldLocation(layer.maxX, layer.maxY),
  )
}

function layerRenderKey(layer: PalworldMapLayer): string {
  return [
    layer.id,
    layer.tileUrlTemplate,
    layer.minZoom,
    layer.maxZoom,
    layer.tileSize,
    layer.transformA,
    layer.transformB,
    layer.transformC,
    layer.transformD,
    layer.minX,
    layer.minY,
    layer.maxX,
    layer.maxY,
  ].join('|')
}

function destroyMap(): void {
  if (renderFrame !== 0) {
    cancelAnimationFrame(renderFrame)
    renderFrame = 0
  }
  actorMarkers.clear()
  actorMarkerRenderKeys.clear()
  actorMarkerStateKeys.clear()
  clusterMarkers.clear()
  clusterMarkerRenderKeys.clear()
  pendingFocus = null
  lastSnapshotKey = ''
  compactActorKeys = new Set<string>()
  mergedActorKeys = new Set<string>()
  actorLayer = null
  if (map !== null) {
    map.remove()
    map = null
  }
}

function createMap(layer: PalworldMapLayer): void {
  if (mapElement.value === null) {
    return
  }
  destroyMap()
  currentLayerKey = layerRenderKey(layer)
  fittedOnce = false
  map = L.map(mapElement.value, {
    crs: buildCRS(layer),
    minZoom: layer.minZoom,
    maxZoom: palworldMapMaxZoom(layer.maxZoom),
    zoomSnap: 0.25,
    // A full level per press, because the range is now three levels longer than
    // the tile source and half-steps made the deep end tedious to reach.
    zoomDelta: 1,
    attributionControl: false,
    preferCanvas: true,
    maxBounds: layerBounds(layer).pad(0.2),
    maxBoundsViscosity: 0.75,
  })
  actorLayer = L.layerGroup().addTo(map)
  map.on('moveend zoomend', scheduleRenderActors)
  if (layer.tileUrlTemplate !== '') {
    L.tileLayer(layer.tileUrlTemplate, {
      tileSize: layer.tileSize,
      minZoom: layer.minZoom,
      maxZoom: palworldMapMaxZoom(layer.maxZoom),
      // Past this level there are no tiles to fetch, so Leaflet upscales the
      // last real one. Terrain softens; the actors drawn over it stay sharp.
      maxNativeZoom: layer.maxZoom,
      noWrap: true,
      bounds: layerBounds(layer),
    }).addTo(map)
  }
  fitWorldBounds()
}

function markerTooltip(actor: PalworldMapActor): HTMLElement {
  const element = document.createElement('span')
  element.textContent = `${actor.name} · X ${formatPalworldCoordinate(actor.locationX)}, Y ${formatPalworldCoordinate(actor.locationY)}`
  return element
}

function actorWorkerCount(actor: PalworldMapActor): number {
  if (actor.kind !== PalworldMapActorKind.BASE) {
    return 0
  }
  if (!visibleKinds.value[PalworldMapActorKind.BASE_WORKER]) {
    return 0
  }
  return baseWorkerAssignment.value.byBase.get(actor.key)?.length ?? 0
}

function createActorMarker(actor: PalworldMapActor): Marker | CircleMarker {
  const category = palworldMapCategory(actor.kind)
  const location = worldLocation(actor.locationX, actor.locationY)
  const color = cssColor(category.colorToken)
  const selected = actor.key === selectedActorKey.value
  let marker: Marker | CircleMarker
  if (category.labeledMarker) {
    const workerCount = actorWorkerCount(actor)
    const countChip =
      workerCount > 0
        ? `<span class="palworld-map-marker__count">${escapeHTML(workerCount.toLocaleString())}</span>`
        : ''
    const modifiers = [
      selected ? ' palworld-map-marker--selected' : '',
      actor.active ? ' palworld-map-marker--active' : '',
      compactActorKeys.has(actor.key) ? ' palworld-map-marker--compact' : '',
    ].join('')
    marker = L.marker(location, {
      icon: L.divIcon({
        className: '',
        html: `<span class="palworld-map-marker palworld-map-marker--${actor.kind}${modifiers}" style="--actor-color:${escapeHTML(color)}"><span class="palworld-map-marker__icon"><span class="material-icons" aria-hidden="true">${escapeHTML(category.icon)}</span></span><span class="palworld-map-marker__label">${escapeHTML(actor.name)}</span>${countChip}</span>`,
        iconAnchor: [17, 17],
      }),
      keyboard: true,
      riseOnHover: true,
      title: actor.name,
      zIndexOffset: actorMarkerZIndex(actor, selected),
    })
  } else if (palworldMapUsesIconMarkers(currentZoom(), nativeMaxZoom())) {
    const size = palworldMapIconSize(currentZoom(), nativeMaxZoom())
    const modifiers = [
      selected ? ' palworld-map-dot--selected' : '',
      actor.active ? '' : ' palworld-map-dot--inactive',
    ].join('')
    marker = L.marker(location, {
      icon: L.divIcon({
        className: '',
        html: `<span class="palworld-map-dot palworld-map-dot--icon palworld-map-dot--${category.shape}${modifiers}" style="--actor-color:${escapeHTML(color)};--dot-size:${size}px"><span class="material-icons" aria-hidden="true">${escapeHTML(category.icon)}</span></span>`,
        iconAnchor: [size / 2, size / 2],
      }),
      keyboard: true,
      riseOnHover: true,
      title: actor.name,
      zIndexOffset: actorMarkerZIndex(actor, selected),
    })
  } else {
    marker = palworldShapeMarker(location, {
      shape: category.shape,
      radius: palworldMapDotRadius(currentZoom(), nativeMaxZoom(), {
        active: actor.active,
        selected,
      }),
      // The stroke is a dark halo rather than the category colour: a 4px dot at
      // 40% fill vanished into the terrain, and the colour now carries in the
      // fill where it has the whole shape to work with.
      color: cssColor('--xy-map-marker-halo'),
      fillColor: color,
      fill: true,
      fillOpacity: palworldMapDotFillOpacity({ active: actor.active, selected }),
      opacity: 0.95,
      weight: selected ? 2.5 : 1.5,
    })
  }
  if (!category.labeledMarker) {
    // Both dot forms carry the coordinate readout, so crossing into icon markers
    // does not silently drop hover detail. Leaflet only invokes function content
    // when the tooltip opens, so this costs nothing until it is read, and the
    // lookup keeps it current for a marker reused across snapshots.
    marker.bindTooltip(() => markerTooltip(actorsByKey.value.get(actor.key) ?? actor), {
      direction: 'top',
      offset: [0, -5],
    })
  }
  marker.on('click', () => {
    selectedActorKey.value = actor.key
    railOpen.value = true
  })
  return marker
}

// Deliberately excludes `active` and the compact flag: both change constantly as
// Pals work and as the viewport moves, and are applied in place instead. Dot
// markers do carry their zoom-derived form, because a canvas shape and a DOM
// icon chip are different objects and the chip bakes its size into its anchor.
function actorMarkerRenderKey(actor: PalworldMapActor): string {
  if (palworldMapCategory(actor.kind).labeledMarker) {
    return [actor.kind, actor.name, actorWorkerCount(actor)].join('|')
  }
  const zoom = currentZoom()
  const native = nativeMaxZoom()
  const iconMode = palworldMapUsesIconMarkers(zoom, native)
  return [actor.kind, actor.name, iconMode, iconMode ? palworldMapIconSize(zoom, native) : 0].join(
    '|',
  )
}

// Labeled markers are DOM overlays, so a large offset reliably wins over the
// latitude ordering Leaflet applies by default.
function actorMarkerZIndex(actor: PalworldMapActor, selected: boolean): number {
  if (selected) {
    return 3_000
  }
  return actor.kind === PalworldMapActorKind.PLAYER ? 2_000 : 1_000
}

function createClusterMarker(cluster: PalworldMapCluster): Marker {
  const category = palworldMapCategory(cluster.kind)
  const color = cssColor(category.colorToken)
  const label = `${cluster.count.toLocaleString()} ${category.label.toLocaleLowerCase()}`
  const modifier = cluster.labelMerge ? ' palworld-map-cluster--merge' : ''
  const marker = L.marker(worldLocation(cluster.locationX, cluster.locationY), {
    icon: L.divIcon({
      className: '',
      html: `<span class="palworld-map-cluster${modifier}" style="--actor-color:${escapeHTML(color)}"><span class="material-icons" aria-hidden="true">${escapeHTML(category.icon)}</span><strong>${cluster.count.toLocaleString()}</strong></span>`,
      // The merge chip is smaller than a density cluster, and its whole job is
      // to mark the exact spot the pills stacked, so it needs its own centre.
      iconAnchor: cluster.labelMerge ? [20, 20] : [24, 24],
    }),
    keyboard: true,
    title: label,
    // A merge stands in for the pills it replaced, so it keeps their height in
    // the stack; a density cluster stays beneath them as before.
    zIndexOffset: cluster.labelMerge ? 1_000 : -1_000,
  })
  marker.on('click', () => {
    if (map === null) {
      return
    }
    // Merged pills are near-coincident, so only the deep end separates them.
    // A density cluster still lands at the level where actors draw singly.
    const targetZoom = cluster.labelMerge
      ? palworldMapMaxZoom(nativeMaxZoom())
      : Math.min(palworldMapMaxZoom(nativeMaxZoom()), Math.max(4, map.getZoom() + 2))
    const firstMember = cluster.actorKeys[0]
    // Actors on the same spot never come apart, however far you zoom. Rather
    // than leave a control that does nothing, the chip hands over to selection,
    // which exempts that actor from merging and peels it out of the stack.
    if (cluster.labelMerge && firstMember !== undefined && map.getZoom() >= targetZoom) {
      selectedActorKey.value = firstMember
      railOpen.value = true
      return
    }
    map.fitBounds(
      L.latLngBounds(
        worldLocation(cluster.minX, cluster.minY),
        worldLocation(cluster.maxX, cluster.maxY),
      ),
      {
        animate: true,
        maxZoom: targetZoom,
        padding: [72, 72],
      },
    )
  })
  return marker
}

function visibleWorldBounds(): {
  minX: number
  minY: number
  maxX: number
  maxY: number
} {
  if (map === null) {
    return {
      minX: activeLayer.value.minX,
      minY: activeLayer.value.minY,
      maxX: activeLayer.value.maxX,
      maxY: activeLayer.value.maxY,
    }
  }
  const bounds = map.getBounds().pad(0.18)
  return {
    minX: bounds.getSouth(),
    minY: bounds.getWest(),
    maxX: bounds.getNorth(),
    maxY: bounds.getEast(),
  }
}

function renderActors(): void {
  if (map === null || actorLayer === null) {
    return
  }
  const activeBounds = layerBounds(activeLayer.value)
  const protectedActorKeys = new Set<string>()
  if (selectedActorKey.value !== '') {
    protectedActorKeys.add(selectedActorKey.value)
  }
  if (
    (search.value.trim() !== '' || focusedGuildKey.value !== '') &&
    filteredActors.value.length <= maxIndividualActors
  ) {
    for (const actor of filteredActors.value) {
      protectedActorKeys.add(actor.key)
    }
  }
  const zoom = map.getZoom()
  const renderPlan = buildPalworldMapRenderPlan(mappedActors.value, {
    zoom,
    bounds: visibleWorldBounds(),
    maxIndividualActors,
    protectedActorKeys,
    selectedActorKey: selectedActorKey.value,
    previousCompactActorKeys: compactActorKeys,
    previousMergedActorKeys: mergedActorKeys,
    project: (actor) => {
      const projected = map?.project(worldLocation(actor.locationX, actor.locationY), zoom)
      return projected ?? { x: actor.locationX, y: actor.locationY }
    },
  })
  compactActorKeys = renderPlan.compactActorKeys
  mergedActorKeys = renderPlan.mergedActorKeys
  aggregatedActorCount.value = renderPlan.aggregatedActorCount
  const visibleActorKeys = new Set<string>()
  for (const actor of renderPlan.actors) {
    const location = worldLocation(actor.locationX, actor.locationY)
    if (!activeBounds.contains(location)) {
      continue
    }
    visibleActorKeys.add(actor.key)
    const renderKey = actorMarkerRenderKey(actor)
    const existingMarker = actorMarkers.get(actor.key)
    if (existingMarker !== undefined && actorMarkerRenderKeys.get(actor.key) === renderKey) {
      existingMarker.setLatLng(location)
      applyActorMarkerState(actor, existingMarker)
      continue
    }
    if (existingMarker !== undefined) {
      actorLayer.removeLayer(existingMarker)
    }
    const marker = createActorMarker(actor)
    marker.addTo(actorLayer)
    actorMarkers.set(actor.key, marker)
    actorMarkerRenderKeys.set(actor.key, renderKey)
    applyActorMarkerState(actor, marker)
  }
  for (const [actorKey, marker] of actorMarkers) {
    if (visibleActorKeys.has(actorKey)) {
      continue
    }
    actorLayer.removeLayer(marker)
    actorMarkers.delete(actorKey)
    actorMarkerRenderKeys.delete(actorKey)
    actorMarkerStateKeys.delete(actorKey)
  }

  const visibleClusterKeys = new Set<string>()
  for (const cluster of renderPlan.clusters) {
    const location = worldLocation(cluster.locationX, cluster.locationY)
    if (!activeBounds.contains(location)) {
      continue
    }
    visibleClusterKeys.add(cluster.key)
    const renderKey = [
      cluster.kind,
      cluster.count,
      cluster.locationX,
      cluster.locationY,
      cluster.minX,
      cluster.minY,
      cluster.maxX,
      cluster.maxY,
    ].join('|')
    const existingMarker = clusterMarkers.get(cluster.key)
    if (existingMarker !== undefined && clusterMarkerRenderKeys.get(cluster.key) === renderKey) {
      existingMarker.setLatLng(location)
      continue
    }
    if (existingMarker !== undefined) {
      actorLayer.removeLayer(existingMarker)
    }
    const marker = createClusterMarker(cluster)
    marker.addTo(actorLayer)
    clusterMarkers.set(cluster.key, marker)
    clusterMarkerRenderKeys.set(cluster.key, renderKey)
  }
  for (const [clusterKey, marker] of clusterMarkers) {
    if (visibleClusterKeys.has(clusterKey)) {
      continue
    }
    actorLayer.removeLayer(marker)
    clusterMarkers.delete(clusterKey)
    clusterMarkerRenderKeys.delete(clusterKey)
  }

  openPendingFocusTooltip()
}

function openPendingFocusTooltip(): void {
  if (pendingFocus === null || map === null) {
    return
  }
  if (pendingFocus.actorKey !== selectedActorKey.value) {
    pendingFocus = null
    return
  }
  const marker = actorMarkers.get(pendingFocus.actorKey)
  if (marker === undefined) {
    return
  }
  marker.openTooltip()
  // Stay armed until the view settles, because a render part-way through the
  // zoom animation still holds the marker form that is about to be replaced.
  if (map.getZoom() >= pendingFocus.zoom) {
    pendingFocus = null
  }
}

function scheduleRenderActors(): void {
  if (renderFrame !== 0) {
    return
  }
  renderFrame = requestAnimationFrame(() => {
    renderFrame = 0
    renderActors()
  })
}

function applyActorMarkerState(actor: PalworldMapActor, marker: Marker | CircleMarker): void {
  const selected = actor.key === selectedActorKey.value
  // Everything applied below derives from this signature, and position is
  // written separately by the caller. Skipping when it is unchanged matters:
  // Marker.setZIndexOffset re-runs a full projection and reposition, so applying
  // it unconditionally doubled the per-frame layout work for every icon marker.
  const stateKey = [selected, actor.active, compactActorKeys.has(actor.key), currentZoom()].join(
    '|',
  )
  if (actorMarkerStateKeys.get(actor.key) === stateKey) {
    return
  }
  actorMarkerStateKeys.set(actor.key, stateKey)
  if (palworldMapCategory(actor.kind).labeledMarker) {
    const markerElement = marker.getElement()?.querySelector('.palworld-map-marker')
    markerElement?.classList.toggle('palworld-map-marker--selected', selected)
    markerElement?.classList.toggle('palworld-map-marker--active', actor.active)
    markerElement?.classList.toggle(
      'palworld-map-marker--compact',
      compactActorKeys.has(actor.key) && !selected,
    )
    if ('setZIndexOffset' in marker) {
      marker.setZIndexOffset(actorMarkerZIndex(actor, selected))
    }
    return
  }
  if (!(marker instanceof L.CircleMarker)) {
    const dotElement = marker.getElement()?.querySelector('.palworld-map-dot')
    dotElement?.classList.toggle('palworld-map-dot--selected', selected)
    dotElement?.classList.toggle('palworld-map-dot--inactive', !actor.active)
    if ('setZIndexOffset' in marker) {
      marker.setZIndexOffset(actorMarkerZIndex(actor, selected))
    }
    return
  }
  marker.setRadius(
    palworldMapDotRadius(currentZoom(), nativeMaxZoom(), { active: actor.active, selected }),
  )
  marker.setStyle({
    fillOpacity: palworldMapDotFillOpacity({ active: actor.active, selected }),
    weight: selected ? 2.5 : 1.5,
  })
}

function refreshActorMarkerState(actorKey: string): void {
  if (actorKey === '') {
    return
  }
  const actor = actorsByKey.value.get(actorKey)
  const marker = actorMarkers.get(actorKey)
  if (actor === undefined || marker === undefined) {
    return
  }
  applyActorMarkerState(actor, marker)
}

// The controller caches snapshots on its own timer, so polling regularly returns
// one that has already been drawn. Without a collection timestamp two snapshots
// cannot be proven identical, so an empty key never matches and always renders.
function snapshotRenderKey(view: PalworldMapView | null): string {
  const collectedAt = view?.collectedAt
  if (view === null || collectedAt === undefined) {
    return ''
  }
  return [
    collectedAt.seconds.toString(),
    collectedAt.nanos,
    view.available,
    view.partial,
    view.stale,
    view.truncated,
  ].join('|')
}

async function refreshMap(): Promise<void> {
  await nextTick()
  const layer = activeLayer.value
  if (map === null || currentLayerKey !== layerRenderKey(layer)) {
    createMap(layer)
    lastSnapshotKey = ''
    map?.invalidateSize({ animate: false })
  }
  const snapshotKey = snapshotRenderKey(props.view)
  if (snapshotKey !== '' && snapshotKey === lastSnapshotKey) {
    return
  }
  lastSnapshotKey = snapshotKey
  renderActors()
  if (!fittedOnce) {
    fitVisibleActors()
  }
}

function fitWorldBounds(): void {
  if (map === null) {
    return
  }
  map.invalidateSize({ animate: false })
  const wideCanvas = (mapElement.value?.clientWidth ?? 0) >= 900
  map.fitBounds(layerBounds(activeLayer.value), {
    animate: false,
    paddingTopLeft: [railOpen.value && wideCanvas ? 320 : 32, 72],
    paddingBottomRight: [32, 48],
  })
  if (hasImagery.value && wideCanvas && map.getZoom() < 0.5 && activeLayer.value.maxZoom >= 1) {
    map.setZoom(0.5, { animate: false })
  }
}

function fitWorld(): void {
  fitWorldBounds()
  fittedOnce = true
}

function fitActors(actorSet: readonly PalworldMapActor[]): void {
  if (map === null) {
    return
  }
  const wideCanvas = (mapElement.value?.clientWidth ?? 0) >= 900
  const positioned = actorSet.filter((actor) =>
    layerBounds(activeLayer.value).contains(worldLocation(actor.locationX, actor.locationY)),
  )
  if (positioned.length === 0) {
    fitWorld()
    return
  }
  map.fitBounds(
    L.latLngBounds(positioned.map((actor) => worldLocation(actor.locationX, actor.locationY))),
    {
      // Stops at the last real tile level. This frames the opening view and
      // category focus, and the extra levels exist to pull crowded actors
      // apart on request, not to be where a sparse world lands on first paint.
      maxZoom: activeLayer.value.maxZoom,
      paddingTopLeft: [railOpen.value && wideCanvas ? 320 : 48, 80],
      paddingBottomRight: [48, 80],
    },
  )
  fittedOnce = true
}

function fitVisibleActors(): void {
  fitActors(filteredActors.value)
}

async function focusActor(actor: PalworldMapActor): Promise<void> {
  visibleKinds.value[actor.kind] = true
  selectedActorKey.value = actor.key
  railOpen.value = true
  await nextTick()
  const location = worldLocation(actor.locationX, actor.locationY)
  // One level short of the ceiling: close enough to read the actor and whatever
  // is around it, without landing on the blurriest imagery.
  const focusZoom = palworldMapFocusZoom(activeLayer.value.maxZoom)
  pendingFocus = { actorKey: actor.key, zoom: focusZoom }
  map?.setView(location, focusZoom, { animate: true })
  openPendingFocusTooltip()
}

async function toggleCategoryFocus(kind: PalworldMapActorKind | null): Promise<void> {
  if (kind === null || focusedKind.value === kind) {
    focusedKind.value = null
    focusedGuildKey.value = ''
    for (const category of availableActorCategories.value) {
      visibleKinds.value[category.kind] = true
    }
    await nextTick()
    fitVisibleActors()
    scheduleRenderActors()
    return
  }
  focusedKind.value = kind
  focusedGuildKey.value = ''
  for (const category of availableActorCategories.value) {
    visibleKinds.value[category.kind] = category.kind === kind
  }
  await nextTick()
  fitVisibleActors()
  scheduleRenderActors()
}

async function focusSelectedGuild(): Promise<void> {
  const base = selectedActor.value
  if (base === null) {
    return
  }
  const guildKey = palworldMapGuildKey(base)
  if (guildKey === '') {
    return
  }
  focusedGuildKey.value = guildKey
  focusedKind.value = null
  search.value = ''
  for (const actor of selectedGuildActors.value) {
    visibleKinds.value[actor.kind] = true
  }
  await nextTick()
  fitActors(selectedGuildActors.value)
  scheduleRenderActors()
}

async function clearGuildFocus(): Promise<void> {
  focusedGuildKey.value = ''
  await nextTick()
  fitVisibleActors()
  scheduleRenderActors()
}

function actorHealthPercent(actor: PalworldMapActor): number {
  if (actor.maxHp <= 0) {
    return 0
  }
  return Math.max(0, Math.min(100, (actor.hp / actor.maxHp) * 100))
}

function toggleActorVisibility(kind: PalworldMapActorKind): void {
  focusedKind.value = null
  visibleKinds.value[kind] = !visibleKinds.value[kind]
}

function selectLayer(layerID: string): void {
  if (activeLayerID.value === layerID) {
    return
  }
  activeLayerID.value = layerID
}

function closeSelectedActor(): void {
  selectedActorKey.value = ''
}

function toggleRail(): void {
  railOpen.value = !railOpen.value
}

async function toggleFullscreen(): Promise<void> {
  if (mapRoot.value === null) {
    return
  }
  try {
    if (document.fullscreenElement === mapRoot.value) {
      await document.exitFullscreen()
    } else {
      await mapRoot.value.requestFullscreen()
    }
  } catch (unknownError: unknown) {
    console.error('Could not change Palworld map fullscreen state', unknownError)
  }
}

function handleFullscreenChange(): void {
  fullscreen.value = document.fullscreenElement === mapRoot.value
  void nextTick(() => map?.invalidateSize({ animate: false }))
}

watch(
  () => props.view,
  () => {
    if (!railInitialized && props.view !== null) {
      railOpen.value = actors.value.length > 0
      railInitialized = true
    }
    if (
      activeLayerID.value === '' ||
      !configuredLayers.value.some((layer) => layer.id === activeLayerID.value)
    ) {
      activeLayerID.value = configuredLayers.value[0]?.id ?? fallbackLayer.id
    }
    void refreshMap()
  },
  { immediate: true },
)
watch(
  () => activeLayerID.value,
  () => void refreshMap(),
)
watch(visibleKinds, () => scheduleRenderActors(), { deep: true })
watch(search, () => scheduleRenderActors())
watch(focusedGuildKey, () => scheduleRenderActors())
watch(selectedActorKey, (actorKey, previousActorKey) => {
  refreshActorMarkerState(previousActorKey)
  refreshActorMarkerState(actorKey)
  scheduleRenderActors()
})

onMounted(() => {
  void refreshMap()
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  if (mapElement.value !== null) {
    resizeObserver = new ResizeObserver(() => map?.invalidateSize({ animate: false }))
    resizeObserver.observe(mapElement.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  destroyMap()
})
</script>

<template>
  <section
    ref="mapRoot"
    class="palworld-live-map"
    :class="{
      'palworld-live-map--rail-open': railOpen,
      'palworld-live-map--public': publicMode,
      'palworld-live-map--imagery': hasImagery,
      'palworld-live-map--fullscreen': fullscreen,
      'palworld-live-map--multiple-layers': configuredLayers.length > 1,
    }">
    <div class="palworld-live-map__canvas-wrap">
      <div ref="mapElement" class="palworld-live-map__canvas" aria-label="Palworld live map" />

      <div class="palworld-live-map__toolbar" role="toolbar" aria-label="Map controls">
        <q-btn
          :aria-label="`${railOpen ? 'Close' : 'Open'} ${actorPanelLabel}`"
          class="palworld-live-map__toolbar-action palworld-live-map__actors-toggle"
          :class="{ 'palworld-live-map__toolbar-action--active': railOpen }"
          flat
          icon="radar"
          round
          @click="toggleRail">
          <q-badge v-if="actors.length > 0" color="primary" floating rounded>
            {{ actors.length > 999 ? '999+' : actors.length }}
          </q-badge>
          <q-tooltip>{{ railOpen ? 'Close' : 'Open' }} {{ actorPanelLabel }}</q-tooltip>
        </q-btn>

        <div
          class="palworld-live-map__status"
          :class="`palworld-live-map__status--${mapStatus.tone}`"
          role="status"
          :aria-label="`${mapStatus.label}. ${collectedLabel}`">
          <q-icon :name="mapStatus.icon" />
          <div>
            <strong>{{ mapStatus.label }}</strong>
            <span :title="collectedTitle">{{ collectedLabel }}</span>
          </div>
        </div>

        <div v-if="configuredLayers.length > 1" class="palworld-live-map__map-layers">
          <button
            v-for="layer in configuredLayers"
            :key="layer.id"
            :class="{ 'palworld-live-map__map-layer--active': activeLayer.id === layer.id }"
            type="button"
            @click="selectLayer(layer.id)">
            {{ layer.label }}
          </button>
        </div>

        <div class="palworld-live-map__toolbar-actions">
          <span v-if="view?.truncated" class="palworld-live-map__toolbar-flag">
            <q-icon name="data_usage" /> Actor limit
          </span>
          <q-btn
            aria-label="Fit world"
            class="palworld-live-map__toolbar-action"
            flat
            icon="center_focus_strong"
            round
            @click="fitWorld">
            <q-tooltip>Fit entire world</q-tooltip>
          </q-btn>
          <q-btn
            :aria-label="fullscreen ? 'Exit fullscreen map' : 'Open fullscreen map'"
            class="palworld-live-map__toolbar-action"
            flat
            :icon="fullscreen ? 'fullscreen_exit' : 'fullscreen'"
            round
            @click="toggleFullscreen">
            <q-tooltip>{{ fullscreen ? 'Exit fullscreen' : 'Fullscreen map' }}</q-tooltip>
          </q-btn>
          <q-btn
            aria-label="Refresh live map"
            class="palworld-live-map__toolbar-action"
            flat
            :loading="loading"
            icon="refresh"
            round
            @click="emit('refresh')">
            <q-tooltip>Refresh live data</q-tooltip>
          </q-btn>
        </div>
      </div>

      <nav
        v-if="view?.available"
        class="palworld-live-map__summaries"
        aria-label="Map population summaries">
        <button
          class="palworld-live-map__summary-chip"
          :class="{
            'palworld-live-map__summary-chip--active':
              focusedKind === null && focusedGuildKey === '',
          }"
          type="button"
          :aria-pressed="focusedKind === null && focusedGuildKey === ''"
          @click="toggleCategoryFocus(null)">
          <q-icon name="public" />
          <span>All</span>
          <strong>{{ countQualifier }}{{ actors.length.toLocaleString() }}</strong>
        </button>
        <button
          v-for="category in summaryCategories"
          :key="category.kind"
          class="palworld-live-map__summary-chip"
          :class="{ 'palworld-live-map__summary-chip--active': focusedKind === category.kind }"
          type="button"
          :aria-label="`Focus ${category.label.toLocaleLowerCase()}`"
          :aria-pressed="focusedKind === category.kind"
          :style="{ '--actor-color': `var(${category.colorToken})` }"
          @click="toggleCategoryFocus(category.kind)">
          <q-icon :name="category.icon" />
          <span>{{ category.label }}</span>
          <strong
            >{{ countQualifier
            }}{{ (actorCounts.get(category.kind) ?? 0).toLocaleString() }}</strong
          >
        </button>
      </nav>

      <div v-if="health !== null" class="palworld-live-map__health" aria-label="World health">
        <span><small>Players</small>{{ health.currentPlayers }} / {{ health.maxPlayers }}</span>
        <span
          ><small>Server</small>{{ health.serverFps.toFixed(1) }} FPS ·
          {{ health.serverFrameTimeMs.toFixed(1) }} ms</span
        >
        <span><small>Bases</small>{{ health.baseCampCount.toLocaleString() }}</span>
        <span><small>World day</small>{{ health.days.toLocaleString() }}</span>
        <span><small>Uptime</small>{{ formatPalworldUptime(health.uptimeSeconds) }}</span>
      </div>

      <div v-if="partialNotice !== ''" class="palworld-live-map__partial" role="status">
        <q-icon name="info" />
        <span>{{ partialNotice }}</span>
      </div>

      <div v-if="aggregatedActorCount > 0" class="palworld-live-map__aggregation" role="status">
        <q-icon name="hub" />
        {{ aggregatedActorCount.toLocaleString() }} actors grouped at this zoom
      </div>

      <aside class="palworld-live-map__rail" :aria-hidden="!railOpen" :inert="!railOpen">
        <div class="palworld-live-map__rail-header">
          <div class="palworld-live-map__rail-heading">
            <span class="palworld-live-map__rail-heading-icon"><q-icon name="radar" /></span>
            <div>
              <div class="palworld-live-map__rail-title">{{ actorPanelTitle }}</div>
              <div class="palworld-live-map__rail-copy">
                {{ actors.length.toLocaleString() }}
                {{ view?.partial ? 'live' : 'reported' }}
              </div>
            </div>
          </div>
          <q-btn
            :aria-label="`Close ${actorPanelLabel}`"
            flat
            icon="close"
            round
            size="sm"
            @click="railOpen = false" />
        </div>

        <div v-if="selectedActor !== null" class="palworld-live-map__selected" role="status">
          <div class="palworld-live-map__selected-heading">
            <span
              class="palworld-live-map__selection-icon"
              :style="{
                '--actor-color': `var(${palworldMapCategory(selectedActor.kind).colorToken})`,
              }">
              <q-icon :name="palworldMapCategory(selectedActor.kind).icon" />
            </span>
            <div>
              <strong>{{ selectedActor.name }}</strong>
              <span>{{ palworldMapCategory(selectedActor.kind).singular }}</span>
            </div>
            <q-btn
              aria-label="Close actor details"
              flat
              icon="close"
              round
              size="sm"
              @click="closeSelectedActor" />
          </div>
          <div class="palworld-live-map__selected-details">
            <span><small>X</small>{{ formatPalworldCoordinate(selectedActor.locationX) }}</span>
            <span><small>Y</small>{{ formatPalworldCoordinate(selectedActor.locationY) }}</span>
            <span><small>Z</small>{{ formatPalworldCoordinate(selectedActor.locationZ) }}</span>
            <span v-if="selectedActor.level > 0"
              ><small>Level</small>{{ selectedActor.level }}</span
            >
            <span><small>Facing</small>{{ formatPalworldFacing(selectedActor.rotationZ) }}</span>
            <span>
              <small>Status</small>
              {{ selectedActor.active ? 'Active' : 'Inactive' }}
            </span>
          </div>
          <div v-if="selectedActor.maxHp > 0" class="palworld-live-map__selected-health">
            <div>
              <span>Health</span>
              <strong
                >{{ selectedActor.hp.toLocaleString() }} /
                {{ selectedActor.maxHp.toLocaleString() }}</strong
              >
            </div>
            <span aria-hidden="true">
              <i :style="{ width: `${actorHealthPercent(selectedActor)}%` }" />
            </span>
          </div>
          <div
            v-if="
              selectedActor.guildName ||
              selectedActor.trainerName ||
              selectedActor.className ||
              selectedActor.action ||
              selectedActor.aiAction
            "
            class="palworld-live-map__selected-meta">
            <span v-if="selectedActor.guildName">Guild · {{ selectedActor.guildName }}</span>
            <span v-if="selectedActor.trainerName">Trainer · {{ selectedActor.trainerName }}</span>
            <span v-if="selectedActor.className">Class · {{ selectedActor.className }}</span>
            <span v-if="selectedActor.action">Action · {{ selectedActor.action }}</span>
            <span v-if="selectedActor.aiAction">AI action · {{ selectedActor.aiAction }}</span>
          </div>

          <section
            v-if="selectedActor.kind === PalworldMapActorKind.BASE"
            class="palworld-live-map__guild-command"
            aria-label="Guilds and bases">
            <div class="palworld-live-map__guild-heading">
              <div>
                <strong>Guilds &amp; bases</strong>
                <span>
                  {{ selectedActor.guildName || 'Unclaimed base' }}
                  <template v-if="selectedGuildUsesNameEstimate">· name-matched estimate</template>
                </span>
              </div>
              <button
                v-if="palworldMapGuildKey(selectedActor) !== ''"
                type="button"
                @click="
                  focusedGuildKey === palworldMapGuildKey(selectedActor)
                    ? clearGuildFocus()
                    : focusSelectedGuild()
                ">
                {{
                  focusedGuildKey === palworldMapGuildKey(selectedActor)
                    ? 'Clear focus'
                    : 'Focus guild'
                }}
              </button>
            </div>
            <dl>
              <div>
                <dt>Bases</dt>
                <dd>
                  {{ countQualifier }}{{ selectedGuildCounts.get(PalworldMapActorKind.BASE) ?? 0 }}
                </dd>
              </div>
              <div>
                <dt>Workers</dt>
                <dd>
                  {{ countQualifier
                  }}{{ selectedGuildCounts.get(PalworldMapActorKind.BASE_WORKER) ?? 0 }}
                </dd>
              </div>
              <div>
                <dt>Players</dt>
                <dd>
                  {{ countQualifier
                  }}{{ selectedGuildCounts.get(PalworldMapActorKind.PLAYER) ?? 0 }}
                </dd>
              </div>
              <div>
                <dt>Companions</dt>
                <dd>
                  {{ countQualifier
                  }}{{ selectedGuildCounts.get(PalworldMapActorKind.COMPANION_PAL) ?? 0 }}
                </dd>
              </div>
            </dl>
            <div class="palworld-live-map__worker-condition">
              <span>
                <small>Active workers</small>
                <strong
                  >{{ countQualifier
                  }}{{ selectedGuildWorkerCondition.active.toLocaleString() }}</strong
                >
              </span>
              <span>
                <small>Injured workers</small>
                <strong
                  >{{ countQualifier
                  }}{{ selectedGuildWorkerCondition.injured.toLocaleString() }}</strong
                >
              </span>
            </div>
            <small v-if="selectedGuildUsesNameEstimate" class="palworld-live-map__guild-estimate">
              Guild identity is unavailable in this snapshot, so related actors are matched by guild
              name.
            </small>
            <div class="palworld-live-map__nearby-workers">
              <span>Base Pals</span>
              <strong>{{ countQualifier }}{{ selectedBaseWorkers.length.toLocaleString() }}</strong>
              <small>
                Nearest workers in this guild; the official API does not report per-base
                assignments.
              </small>
            </div>
            <ul v-if="selectedBaseWorkers.length > 0" class="palworld-live-map__base-pals">
              <li v-for="worker in selectedBaseWorkers" :key="worker.key">
                <button type="button" @click="focusActor(worker)">
                  <span
                    class="palworld-live-map__base-pal-icon"
                    :class="{
                      'palworld-live-map__base-pal-icon--injured':
                        worker.maxHp > 0 && worker.hp < worker.maxHp,
                    }">
                    <q-icon :name="palworldMapCategory(worker.kind).icon" />
                  </span>
                  <span class="palworld-live-map__base-pal-name">{{ worker.name }}</span>
                  <span v-if="worker.maxHp > 0" class="palworld-live-map__base-pal-health">
                    {{ Math.round(actorHealthPercent(worker)) }}%
                  </span>
                  <span v-if="!worker.active" class="palworld-live-map__base-pal-idle">Idle</span>
                </button>
              </li>
            </ul>
          </section>
        </div>

        <template v-else>
          <q-input
            v-if="actors.length > 0"
            v-model="search"
            :aria-label="`Search ${actorPanelLabel}`"
            class="palworld-live-map__search"
            clearable
            dense
            outlined
            :placeholder="view?.partial ? 'Search players' : 'Search actors'">
            <template #prepend><q-icon name="search" /></template>
          </q-input>

          <div class="palworld-live-map__layers" aria-label="Actor layers">
            <button
              v-for="category in moreKindsOpen ? availableActorCategories : primaryActorCategories"
              :key="category.kind"
              class="palworld-live-map__layer"
              :class="{ 'palworld-live-map__layer--active': visibleKinds[category.kind] }"
              type="button"
              :aria-pressed="Boolean(visibleKinds[category.kind])"
              @click="toggleActorVisibility(category.kind)">
              <span
                class="palworld-live-map__layer-icon"
                :style="{ '--actor-color': `var(${category.colorToken})` }">
                <q-icon :name="category.icon" />
              </span>
              <span class="palworld-live-map__layer-label">{{ category.label }}</span>
              <span class="palworld-live-map__layer-count">{{
                actorCounts.get(category.kind) ?? 0
              }}</span>
              <q-icon
                :aria-label="visibleKinds[category.kind] ? 'Visible' : 'Hidden'"
                :name="visibleKinds[category.kind] ? 'visibility' : 'visibility_off'"
                size="17px" />
            </button>
            <button
              v-if="secondaryActorCategories.length > 0"
              class="palworld-live-map__more-layers"
              type="button"
              :aria-expanded="moreKindsOpen"
              @click="moreKindsOpen = !moreKindsOpen">
              <span>More actors</span>
              <span>{{ secondaryActorCount.toLocaleString() }}</span>
              <q-icon :name="moreKindsOpen ? 'expand_less' : 'expand_more'" size="18px" />
            </button>
          </div>

          <template v-if="actors.length > 0">
            <div class="palworld-live-map__roster-heading">
              <span>Visible actors</span>
              <span>{{ filteredActors.length.toLocaleString() }}</span>
            </div>
            <q-virtual-scroll
              v-if="filteredActors.length > 0"
              class="palworld-live-map__roster"
              :items="filteredActors"
              :virtual-scroll-item-size="54">
              <template #default="{ item: actor }">
                <button
                  :key="actor.key"
                  class="palworld-live-map__actor"
                  :class="{ 'palworld-live-map__actor--selected': selectedActorKey === actor.key }"
                  type="button"
                  @click="focusActor(actor)">
                  <span
                    class="palworld-live-map__actor-symbol"
                    :style="{
                      '--actor-color': `var(${palworldMapCategory(actor.kind).colorToken})`,
                    }">
                    <q-icon :name="palworldMapCategory(actor.kind).icon" />
                  </span>
                  <span class="palworld-live-map__actor-copy">
                    <strong>{{ actor.name }}</strong>
                    <span>
                      {{ palworldMapCategory(actor.kind).singular }} · X
                      {{ formatPalworldCoordinate(actor.locationX) }} · Y
                      {{ formatPalworldCoordinate(actor.locationY) }}
                    </span>
                  </span>
                </button>
              </template>
            </q-virtual-scroll>
            <div v-else class="palworld-live-map__roster-empty">
              <q-icon name="filter_alt_off" size="24px" />
              <strong>No matching actors</strong>
              <span>Change the search or enable another actor type.</span>
            </div>
          </template>
          <div
            v-else
            class="palworld-live-map__roster-empty palworld-live-map__roster-empty--world">
            <q-icon name="sensors_off" size="28px" />
            <strong>{{
              view?.partial ? 'No players reported' : 'No world actors reported'
            }}</strong>
            <span>
              {{
                view?.partial
                  ? 'Player positions will appear while players are online.'
                  : 'Map imagery remains available. Actors will appear when the server provides a snapshot.'
              }}
            </span>
          </div>
        </template>
      </aside>

      <div
        v-if="view?.unavailableReason"
        class="palworld-live-map__notice"
        :class="{ 'palworld-live-map__notice--overlay': view.available }">
        <q-icon :name="view.available ? 'history' : 'map'" />
        <span>{{ view.unavailableReason }}</span>
      </div>

      <div class="palworld-live-map__footer">
        <span v-if="!hasImagery">Coordinate grid · no map imagery configured</span>
        <span v-else>{{ activeLayer.attribution }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.palworld-live-map {
  position: relative;
  display: block;
  flex: 1 0 auto;
  min-height: 520px;
  height: calc(100dvh - 240px);
  overflow: hidden;
  background: var(--xy-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.palworld-live-map--public {
  height: calc(100dvh - 112px);
}

.palworld-live-map--fullscreen {
  width: 100vw;
  height: 100dvh;
  border: 0;
  border-radius: 0;
}

.palworld-live-map__rail {
  position: absolute;
  z-index: var(--xy-z-drawer);
  top: 132px;
  bottom: 94px;
  left: var(--xy-space-base);
  display: flex;
  flex-direction: column;
  width: 288px;
  min-height: 0;
  overflow: hidden;
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-xl);
  box-shadow: var(--xy-shadow-md);
  opacity: 0;
  pointer-events: none;
  transform: translateX(calc(-100% - var(--xy-space-lg)));
  backdrop-filter: blur(10px);
  transition:
    opacity var(--xy-transition-fast),
    transform var(--xy-transition-base);
}

.palworld-live-map--rail-open .palworld-live-map__rail {
  opacity: 1;
  pointer-events: auto;
  transform: translateX(0);
}

.palworld-live-map__rail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-height: 58px;
  padding: var(--xy-space-sm) var(--xy-space-base);
  border-bottom: 1px solid var(--xy-border);
}

.palworld-live-map__rail-heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.palworld-live-map__rail-heading-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-md);
}

.palworld-live-map__rail-title {
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  color: var(--xy-text-primary);
}

.palworld-live-map__rail-copy,
.palworld-live-map__actor-copy span,
.palworld-live-map__roster-empty span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__search {
  margin: var(--xy-space-base) var(--xy-space-base) var(--xy-space-sm);
}

.palworld-live-map__layers {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm) var(--xy-space-sm) var(--xy-space-base);
}

.palworld-live-map__layer,
.palworld-live-map__actor {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 40px;
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  text-align: left;
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.palworld-live-map__layer {
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-base) var(--xy-space-xs) var(--xy-space-sm);
}

.palworld-live-map__layer:hover,
.palworld-live-map__layer:focus-visible,
.palworld-live-map__actor:hover,
.palworld-live-map__actor:focus-visible {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  outline: none;
}

.palworld-live-map__layer--active {
  color: var(--xy-text-primary);
  background: var(--xy-surface-overlay-soft);
}

.palworld-live-map__layer-icon,
.palworld-live-map__actor-symbol,
.palworld-live-map__selection-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  color: var(--actor-color);
}

.palworld-live-map__layer-icon {
  width: 26px;
  height: 26px;
  background: color-mix(in srgb, var(--actor-color) 12%, transparent);
  border-radius: var(--xy-radius-md);
}

.palworld-live-map__layer-label {
  flex: 1;
  font-weight: 600;
}

.palworld-live-map__layer-count {
  min-width: 28px;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  text-align: right;
}

.palworld-live-map__more-layers {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 32px;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font: inherit;
  font-size: var(--xy-font-size-xs);
  text-align: left;
}

.palworld-live-map__more-layers:hover,
.palworld-live-map__more-layers:focus-visible {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  outline: none;
}

.palworld-live-map__more-layers span:nth-child(2) {
  font-family: var(--xy-font-mono);
}

.palworld-live-map__roster-heading {
  display: flex;
  justify-content: space-between;
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  border-top: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
}

.palworld-live-map__selected {
  display: grid;
  flex: 1;
  gap: var(--xy-space-base);
  min-height: 0;
  margin: var(--xy-space-base);
  padding: var(--xy-space-base);
  overflow-x: hidden;
  overflow-y: auto;
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-lg);
}

.palworld-live-map__selected-heading {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-sm);
}

.palworld-live-map__selected-heading > div {
  display: grid;
  min-width: 0;
}

.palworld-live-map__selected-heading > div strong,
.palworld-live-map__selected-heading > div span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__selected-heading > div strong {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.palworld-live-map__selected-heading > div span,
.palworld-live-map__selected-meta {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selected-details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-xs);
}

.palworld-live-map__selected-details > span {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-xs);
  min-width: 0;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border-radius: var(--xy-radius-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selected-details small {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
}

.palworld-live-map__selected-meta {
  display: grid;
  gap: var(--xy-space-2xs);
}

.palworld-live-map__selected-health {
  display: grid;
  gap: var(--xy-space-xs);
}

.palworld-live-map__selected-health > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selected-health strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.palworld-live-map__selected-health > span {
  height: 5px;
  overflow: hidden;
  background: var(--xy-surface-4);
  border-radius: var(--xy-radius-pill);
}

.palworld-live-map__selected-health i {
  display: block;
  height: 100%;
  background: var(--xy-success);
  border-radius: inherit;
}

.palworld-live-map__guild-command {
  display: grid;
  gap: var(--xy-space-sm);
  padding-top: var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
}

.palworld-live-map__guild-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.palworld-live-map__guild-heading > div {
  display: grid;
  min-width: 0;
}

.palworld-live-map__guild-heading strong {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.palworld-live-map__guild-heading span,
.palworld-live-map__nearby-workers small {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__guild-heading button {
  min-height: 44px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border: 1px solid var(--xy-accent-border);
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font: inherit;
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
}

.palworld-live-map__guild-heading button:hover,
.palworld-live-map__guild-heading button:focus-visible {
  color: var(--xy-text-primary);
  border-color: var(--xy-accent);
  outline: none;
}

.palworld-live-map__guild-command dl {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-xs);
  margin: 0;
}

.palworld-live-map__guild-command dl > div {
  display: grid;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-xs);
  background: var(--xy-surface-1);
  border-radius: var(--xy-radius-sm);
  text-align: center;
}

.palworld-live-map__guild-command dt {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
}

.palworld-live-map__guild-command dd {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__nearby-workers {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: baseline;
  gap: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__base-pals {
  display: grid;
  gap: var(--xy-space-2xs);
  max-height: 220px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.palworld-live-map__base-pals button {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--xy-space-sm);
  width: 100%;
  padding: var(--xy-space-2xs) var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  text-align: left;
  background: none;
  border: 1px solid transparent;
  border-radius: var(--xy-radius-sm);
  cursor: pointer;
}

.palworld-live-map__base-pals button:hover {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border-color: var(--xy-border-hover);
}

.palworld-live-map__base-pal-icon {
  display: inline-flex;
  color: var(--xy-success);
}

.palworld-live-map__base-pal-icon--injured {
  color: var(--xy-danger);
}

.palworld-live-map__base-pal-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__base-pal-health,
.palworld-live-map__base-pal-idle {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
}

.palworld-live-map__worker-condition {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-xs);
}

.palworld-live-map__worker-condition > span {
  display: grid;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-1);
  border-radius: var(--xy-radius-sm);
}

.palworld-live-map__worker-condition small,
.palworld-live-map__guild-estimate {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
}

.palworld-live-map__worker-condition strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__guild-estimate {
  line-height: var(--xy-line-height-base);
}

.palworld-live-map__nearby-workers strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.palworld-live-map__nearby-workers small {
  grid-column: 1 / -1;
}

.palworld-live-map__roster {
  flex: 1;
  min-height: 0;
}

.palworld-live-map__actor {
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-radius: 0;
}

.palworld-live-map__actor--selected {
  background: var(--xy-primary-bg-subtle);
}

.palworld-live-map__actor-symbol {
  width: 28px;
  height: 28px;
  border: 1px solid color-mix(in srgb, var(--actor-color) 52%, transparent);
  border-radius: var(--xy-radius-pill);
}

.palworld-live-map__actor-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.palworld-live-map__actor-copy strong {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__roster-empty {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}

.palworld-live-map__roster-empty--world {
  margin: auto 0;
  padding-block: var(--xy-space-lg);
}

.palworld-live-map__roster-empty--world .q-icon {
  color: var(--xy-accent);
}

.palworld-live-map__roster-empty strong {
  color: var(--xy-text-primary);
}

.palworld-live-map__canvas-wrap {
  position: absolute;
  inset: 0;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.palworld-live-map__canvas {
  width: 100%;
  height: 100%;
  background-color: var(--xy-base);
  background-image:
    linear-gradient(var(--xy-chart-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--xy-chart-grid) 1px, transparent 1px);
  background-size: 32px 32px;
}

.palworld-live-map--imagery .palworld-live-map__canvas {
  background-color: color-mix(in srgb, var(--xy-base) 84%, var(--xy-info) 16%);
  background-image: radial-gradient(
    circle at 50% 46%,
    color-mix(in srgb, var(--xy-base) 76%, var(--xy-info) 24%) 0,
    color-mix(in srgb, var(--xy-base) 88%, var(--xy-info) 12%) 62%,
    var(--xy-base) 100%
  );
  background-size: auto;
}

.palworld-live-map__toolbar {
  position: absolute;
  z-index: var(--xy-z-drawer);
  top: var(--xy-space-base);
  left: var(--xy-space-base);
  right: var(--xy-space-base);
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 52px;
  padding: var(--xy-space-xs);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-xl);
  box-shadow: var(--xy-shadow-md);
  backdrop-filter: blur(10px);
}

.palworld-live-map__summaries,
.palworld-live-map__health {
  position: absolute;
  z-index: var(--xy-z-drawer);
  right: var(--xy-space-base);
  left: var(--xy-space-base);
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  overflow-x: auto;
  scrollbar-width: none;
}

.palworld-live-map__summaries::-webkit-scrollbar,
.palworld-live-map__health::-webkit-scrollbar {
  display: none;
}

.palworld-live-map__summaries {
  top: 76px;
}

.palworld-live-map__summary-chip {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 44px;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-secondary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-pill);
  box-shadow: var(--xy-shadow-sm);
  cursor: pointer;
  font: inherit;
  font-size: var(--xy-font-size-xs);
  backdrop-filter: blur(8px);
}

.palworld-live-map__summary-chip > .q-icon {
  color: var(--actor-color, var(--xy-accent));
  font-size: 18px;
}

.palworld-live-map__summary-chip strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.palworld-live-map__summary-chip:hover,
.palworld-live-map__summary-chip:focus-visible,
.palworld-live-map__summary-chip--active {
  color: var(--xy-text-primary);
  background: var(--xy-surface-3);
  border-color: color-mix(in srgb, var(--actor-color, var(--xy-accent)) 58%, var(--xy-border) 42%);
  outline: none;
}

.palworld-live-map__health {
  bottom: 42px;
  justify-content: flex-end;
  pointer-events: none;
}

.palworld-live-map__health > span {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: baseline;
  gap: var(--xy-space-xs);
  min-height: 36px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  backdrop-filter: blur(8px);
}

.palworld-live-map__health small {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
}

.palworld-live-map__aggregation {
  position: absolute;
  z-index: var(--xy-z-drawer);
  bottom: 86px;
  left: var(--xy-space-base);
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 32px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-text-secondary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-sm);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__partial {
  position: absolute;
  z-index: var(--xy-z-drawer);
  bottom: 126px;
  left: var(--xy-space-base);
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-xs);
  max-width: min(420px, calc(100% - 2 * var(--xy-space-base)));
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-warning);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-sm);
  font-size: var(--xy-font-size-xs);
  line-height: var(--xy-line-height-tight);
}

.palworld-live-map__partial > .q-icon {
  flex: 0 0 auto;
  margin-top: 1px;
  color: var(--xy-warning);
  font-size: 16px;
}

.palworld-live-map__toolbar-action {
  color: var(--xy-text-secondary);
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.palworld-live-map__toolbar-action:hover,
.palworld-live-map__toolbar-action:focus-visible,
.palworld-live-map__toolbar-action--active {
  color: var(--xy-text-primary);
  background: var(--xy-surface-3);
}

.palworld-live-map__status {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 154px;
  padding: 0 var(--xy-space-sm);
}

.palworld-live-map__status > .q-icon {
  font-size: 22px;
}

.palworld-live-map__status div {
  display: flex;
  flex-direction: column;
}

.palworld-live-map__status strong {
  font-size: var(--xy-font-size-sm);
}

.palworld-live-map__status span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__status--success > .q-icon {
  color: var(--xy-success);
}

.palworld-live-map__status--warning > .q-icon {
  color: var(--xy-warning);
}

.palworld-live-map__status--info > .q-icon {
  color: var(--xy-info);
}

.palworld-live-map__status--danger > .q-icon {
  color: var(--xy-danger);
}

.palworld-live-map__map-layers {
  display: flex;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-xs);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-lg);
}

.palworld-live-map__map-layers button {
  min-height: 34px;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font: inherit;
}

.palworld-live-map__map-layers button:hover,
.palworld-live-map__map-layers button:focus-visible,
.palworld-live-map__map-layer--active {
  color: var(--xy-text-primary) !important;
  background: var(--xy-surface-3) !important;
  outline: none;
}

.palworld-live-map__toolbar-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-2xs);
  margin-left: auto;
}

.palworld-live-map__toolbar-flag {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 30px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-warning);
  background: var(--xy-warning-bg-faint);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__notice,
.palworld-live-map__footer {
  position: absolute;
  z-index: var(--xy-z-drawer);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
}

.palworld-live-map__notice {
  top: 50%;
  left: 50%;
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  max-width: min(520px, calc(100% - 48px));
  padding: var(--xy-space-md);
  border-radius: var(--xy-radius-lg);
  transform: translate(-50%, -50%);
  text-align: center;
}

.palworld-live-map__notice--overlay {
  top: 84px;
  transform: translateX(-50%);
}

.palworld-live-map__selection-icon {
  width: 40px;
  height: 40px;
  background: color-mix(in srgb, var(--actor-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 45%, transparent);
  border-radius: var(--xy-radius-lg);
  font-size: 22px;
}

.palworld-live-map__footer {
  right: var(--xy-space-sm);
  bottom: var(--xy-space-sm);
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 26px;
  max-width: calc(100% - var(--xy-space-md));
  padding: 0 var(--xy-space-sm);
  overflow: hidden;
  background: color-mix(in srgb, var(--xy-surface-1) 88%, transparent);
  border-color: var(--xy-border);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .palworld-live-map {
    min-height: 500px;
  }

  .palworld-live-map__rail {
    z-index: var(--xy-z-drawer);
    top: 132px;
    bottom: 94px;
    width: min(86vw, 300px);
  }

  .palworld-live-map__status {
    min-width: 0;
  }

  .palworld-live-map__summaries,
  .palworld-live-map__health {
    padding-bottom: var(--xy-space-2xs);
  }

  .palworld-live-map__health {
    justify-content: flex-start;
    pointer-events: auto;
  }
}

@media (max-width: 700px) {
  .palworld-live-map {
    min-height: 440px;
    height: calc(100dvh - 300px);
  }

  .palworld-live-map__toolbar {
    top: var(--xy-space-sm);
    left: var(--xy-space-sm);
    right: var(--xy-space-sm);
    flex-wrap: wrap;
  }

  .palworld-live-map__status {
    flex: 1;
  }

  .palworld-live-map__map-layers {
    order: 4;
    width: 100%;
    overflow-x: auto;
  }

  .palworld-live-map__map-layers button {
    flex: 1 0 auto;
  }

  .palworld-live-map__rail {
    top: 210px;
    bottom: var(--xy-space-sm);
    left: var(--xy-space-sm);
    width: min(300px, calc(100% - 2 * var(--xy-space-sm)));
  }

  .palworld-live-map__summaries {
    top: 122px;
    right: var(--xy-space-sm);
    left: var(--xy-space-sm);
  }

  .palworld-live-map--multiple-layers .palworld-live-map__summaries {
    top: 154px;
  }

  .palworld-live-map__health {
    right: var(--xy-space-sm);
    bottom: 38px;
    left: var(--xy-space-sm);
  }

  .palworld-live-map__aggregation {
    bottom: 88px;
    left: var(--xy-space-sm);
  }

  .palworld-live-map__partial {
    bottom: 128px;
    left: var(--xy-space-sm);
    max-width: calc(100% - 2 * var(--xy-space-sm));
  }

  .palworld-live-map__roster-empty--world {
    padding-block: var(--xy-space-base);
  }

  .palworld-live-map__roster-empty--world span {
    display: none;
  }

  .palworld-live-map__layer,
  .palworld-live-map__more-layers {
    min-height: 44px;
  }

  .palworld-live-map__rail .q-btn {
    width: 44px;
    min-width: 44px;
    min-height: 44px;
  }
}

@media (max-width: 360px) {
  .palworld-live-map__status {
    display: block;
    min-width: 0;
    padding-inline: var(--xy-space-2xs);
    overflow: hidden;
  }

  .palworld-live-map__status > .q-icon,
  .palworld-live-map__status span {
    display: none;
  }

  .palworld-live-map__status strong {
    font-size: var(--xy-font-size-xs);
    line-height: var(--xy-line-height-tight);
  }

  .palworld-live-map__toolbar-actions .q-btn {
    width: 44px;
    min-width: 44px;
    min-height: 44px;
  }
}
</style>

<style>
.palworld-map-cluster {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-xs);
  min-width: 48px;
  height: 48px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 97%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 72%, var(--xy-border) 28%);
  border-radius: var(--xy-radius-pill);
  box-shadow: var(--xy-shadow-lg);
  font-family: var(--xy-font-mono);
}

.palworld-map-cluster .material-icons {
  color: var(--actor-color);
  font-size: 18px;
}

/* A merge replaces pills that would have stacked, so it borrows their weight
   rather than the quieter density-cluster treatment. */
.palworld-map-cluster--merge {
  min-width: 40px;
  height: 40px;
  border-color: var(--actor-color);
}

/* Past the tile source the viewport holds few enough actors to spend a DOM node
   each, and the category icon says far more than a coloured dot can. */
.palworld-map-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--dot-size);
  height: var(--dot-size);
  color: var(--actor-color);
  background: color-mix(in srgb, var(--xy-surface-1) 92%, var(--actor-color) 8%);
  border: 1px solid color-mix(in srgb, var(--actor-color) 68%, var(--xy-border) 32%);
  border-radius: var(--xy-radius-pill);
  box-shadow: 0 0 0 1px var(--xy-map-marker-halo);
  transition:
    transform var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
}

.palworld-map-dot .material-icons {
  font-size: calc(var(--dot-size) * 0.62);
}

/* Silhouette carries the kind where colour alone failed at dot size. */
.palworld-map-dot--diamond {
  border-radius: var(--xy-radius-sm);
  transform: rotate(45deg);
}

.palworld-map-dot--diamond .material-icons {
  transform: rotate(-45deg);
}

.palworld-map-dot--square {
  border-radius: var(--xy-radius-sm);
}

.palworld-map-dot--inactive {
  opacity: 0.62;
}

.palworld-map-dot--selected {
  border-color: var(--actor-color);
  box-shadow:
    0 0 0 1px var(--xy-map-marker-halo),
    0 0 0 3px color-mix(in srgb, var(--actor-color) 45%, transparent);
}

.palworld-map-dot--selected.palworld-map-dot--diamond {
  transform: rotate(45deg) scale(1.1);
}

.palworld-map-dot--selected:not(.palworld-map-dot--diamond) {
  transform: scale(1.1);
}

.palworld-map-marker {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 34px;
  padding: 4px 9px 4px 4px;
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 97%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 58%, var(--xy-border) 42%);
  border-radius: var(--xy-radius-pill);
  box-shadow: var(--xy-shadow-md);
  white-space: nowrap;
  transform-origin: 17px 17px;
  transition:
    transform var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
}

.palworld-map-marker__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  color: var(--actor-color);
  font-size: 17px;
  background: color-mix(in srgb, var(--actor-color) 13%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 42%, transparent);
  border-radius: var(--xy-radius-pill);
}

.palworld-map-marker--selected {
  border-color: var(--actor-color);
  transform: scale(1.08);
}

.palworld-map-marker--active::after {
  position: absolute;
  top: 4px;
  left: 4px;
  width: 26px;
  height: 26px;
  border: 1px solid var(--actor-color);
  border-radius: var(--xy-radius-pill);
  content: '';
  opacity: 0.4;
  pointer-events: none;
  transform: scale(1.3);
}

/* Only the selected marker pulses. An infinite animation on every active marker
   keeps the compositor rendering frames for as long as the map stays open. */
.palworld-map-marker--active.palworld-map-marker--selected::after {
  animation: palworld-marker-signal calc(1.8s * var(--xy-animation-duration)) ease-out infinite;
}

@keyframes palworld-marker-signal {
  0% {
    opacity: 0.7;
    transform: scale(0.82);
  }

  75%,
  100% {
    opacity: 0;
    transform: scale(1.65);
  }
}

.palworld-map-marker__label {
  max-width: 180px;
  overflow: hidden;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  text-overflow: ellipsis;
  transition:
    max-width var(--xy-transition-fast),
    opacity var(--xy-transition-fast);
}

.palworld-map-marker__count {
  padding: 0 var(--xy-space-xs);
  color: var(--actor-color);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
  font-weight: 700;
  background: color-mix(in srgb, var(--actor-color) 15%, transparent);
  border-radius: var(--xy-radius-pill);
}

/* A stack of bases collapses to icons so one name stays readable; hovering or
   selecting any of them brings its own name back. */
.palworld-map-marker--compact {
  gap: 0;
  padding: 4px;
}

.palworld-map-marker--compact .palworld-map-marker__label,
.palworld-map-marker--compact .palworld-map-marker__count {
  max-width: 0;
  padding: 0;
  overflow: hidden;
  opacity: 0;
}

.palworld-map-marker--compact:hover {
  z-index: 1;
  gap: var(--xy-space-xs);
  padding: 4px 9px 4px 4px;
}

.palworld-map-marker--compact:hover .palworld-map-marker__label {
  max-width: 180px;
  opacity: 1;
}

.palworld-map-marker--compact:hover .palworld-map-marker__count {
  max-width: 180px;
  padding: 0 var(--xy-space-xs);
  opacity: 1;
}

.palworld-live-map .leaflet-control-zoom a {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border-color: var(--xy-border-strong);
}

.palworld-live-map .leaflet-top.leaflet-left {
  top: 128px;
  right: var(--xy-space-base);
  left: auto;
}

.palworld-live-map .leaflet-control-zoom {
  border: 1px solid var(--xy-border-hover);
  box-shadow: var(--xy-shadow-md);
}

.palworld-live-map .leaflet-tooltip {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-md);
  font-family: var(--xy-font-body);
}

.palworld-live-map .leaflet-tooltip::before {
  border-top-color: var(--xy-border-strong);
}

@media (max-width: 700px) {
  .palworld-live-map .leaflet-top.leaflet-left {
    top: 170px;
    right: var(--xy-space-sm);
  }
}
</style>
