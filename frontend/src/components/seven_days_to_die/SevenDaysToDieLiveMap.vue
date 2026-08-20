<script lang="ts" setup>
import { timestampDate } from '@bufbuild/protobuf/wkt'
import L, { type DoneCallback, type Map as LeafletMap } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import type { SevenDaysToDieMapPlayer, SevenDaysToDieMapView } from '@/proto/xylona_pb'
import {
  formatSevenDaysToDieCoordinate,
  initialSevenDaysToDieMapView,
  sevenDaysToDieTileURL,
} from '@/pages/game_servers/seven-days-to-die-map'

const fullTileRefreshIntervalMilliseconds = 30_000

const props = withDefaults(
  defineProps<{
    view: SevenDaysToDieMapView | null
    loading?: boolean
    loadError?: boolean
    publicMode?: boolean
    shareToken?: string
  }>(),
  {
    loading: false,
    loadError: false,
    publicMode: false,
    shareToken: '',
  },
)

const emit = defineEmits<{
  refresh: []
}>()

const mapElement = ref<HTMLElement | null>(null)

let map: LeafletMap | null = null
let tileLayer: AuthorizedTileLayer | null = null
let playerLayer: L.LayerGroup | null = null
let resizeObserver: ResizeObserver | null = null
let initializedKey = ''
let lastFullTileRefreshAt = Date.now()

const players = computed(() => props.view?.players ?? [])
const onlinePlayers = computed(() => players.value.filter((player) => player.online).length)
const mapStatus = computed(() => {
  if (props.loadError) {
    return { label: 'Connection lost', icon: 'cloud_off', tone: 'danger' }
  }
  if (props.loading && props.view === null) {
    return { label: 'Loading world', icon: 'sync', tone: 'info' }
  }
  if (!props.view?.enabled) {
    return { label: 'Map unavailable', icon: 'map', tone: 'warning' }
  }
  if (props.view.stale) {
    return { label: 'Last known world', icon: 'history', tone: 'warning' }
  }
  return { label: 'Live world', icon: 'sensors', tone: 'success' }
})
const collectedLabel = computed(() => {
  const collectedAt = props.view?.collectedAt
  if (collectedAt === undefined) {
    return 'No snapshot received yet'
  }
  return `Updated ${timestampDate(collectedAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })}`
})

class AuthorizedTileLayer extends L.GridLayer {
  private readonly template: string
  private readonly shareToken: string

  constructor(template: string, shareToken: string, options: L.GridLayerOptions) {
    super(options)
    this.template = template
    this.shareToken = shareToken
  }

  protected override createTile(coordinates: L.Coords, done: DoneCallback): HTMLElement {
    const image = document.createElement('img')
    image.alt = ''
    image.setAttribute('role', 'presentation')

    this.loadTile(coordinates, image, done)
    return image
  }

  refreshPlayerTiles(map: LeafletMap, players: readonly SevenDaysToDieMapPlayer[]): void {
    const tileSize = this.getTileSize()
    for (const tile of Object.values(this._tiles)) {
      if (!tile.current) {
        continue
      }
      const containsOnlinePlayer = players.some((player) => {
        if (!player.online || player.position === undefined) {
          return false
        }
        const projected = map
          .project([player.position.x, player.position.z], tile.coords.z)
          .unscaleBy(tileSize)
          .floor()
        return (
          Math.abs(projected.x - tile.coords.x) <= 1 && Math.abs(projected.y - tile.coords.y) <= 1
        )
      })
      if (containsOnlinePlayer) {
        this.loadTile(tile.coords, tile.el as HTMLImageElement)
      }
    }
  }

  refreshAllTiles(): void {
    for (const tile of Object.values(this._tiles)) {
      if (tile.current) {
        this.loadTile(tile.coords, tile.el as HTMLImageElement)
      }
    }
  }

  private loadTile(coordinates: L.Coords, image: HTMLImageElement, done?: DoneCallback): void {
    const headers: HeadersInit = { Accept: 'image/png' }
    if (this.shareToken !== '') {
      headers['X-Xylona-Map-Share'] = this.shareToken
    }

    void fetch(sevenDaysToDieTileURL(this.template, coordinates), {
      cache: 'no-store',
      credentials: 'same-origin',
      headers,
    })
      .then((response) => {
        if (!response.ok) {
          throw new Error(`Map tile request failed with status ${response.status}.`)
        }
        return response.blob()
      })
      .then((blob) => {
        const objectURL = URL.createObjectURL(blob)
        image.addEventListener(
          'load',
          () => {
            URL.revokeObjectURL(objectURL)
            done?.(undefined, image)
          },
          { once: true },
        )
        image.addEventListener(
          'error',
          () => {
            URL.revokeObjectURL(objectURL)
            done?.(new Error('The map tile could not be decoded.'), image)
          },
          { once: true },
        )
        image.src = objectURL
      })
      .catch((error: unknown) => {
        done?.(
          error instanceof Error ? error : new Error('The map tile could not be loaded.'),
          image,
        )
      })
  }
}

function createSevenDaysToDieCRS(maxZoom: number): L.CRS {
  const coordinateScale = 2 ** maxZoom
  const projection: L.Projection = {
    project(latLng: L.LatLng): L.Point {
      return new L.Point(latLng.lat / coordinateScale, latLng.lng / coordinateScale)
    },
    unproject(point: L.Point): L.LatLng {
      return new L.LatLng(point.x * coordinateScale, point.y * coordinateScale)
    },
    bounds: L.bounds([-coordinateScale, -coordinateScale], [coordinateScale, coordinateScale]),
  }

  return L.extend({}, L.CRS.Simple, {
    projection,
    transformation: new L.Transformation(1, 0, -1, 0),
    scale(zoom: number): number {
      return 2 ** zoom
    },
    zoom(scale: number): number {
      return Math.log(scale) / Math.LN2
    },
    infinite: true,
  }) as L.CRS
}

function coordinateText(x: number, y: number, z: number): string {
  return `X ${formatSevenDaysToDieCoordinate(x)} · Y ${formatSevenDaysToDieCoordinate(y)} · Z ${formatSevenDaysToDieCoordinate(z)}`
}

function createPlayerPopup(player: SevenDaysToDieMapPlayer): HTMLElement {
  const popup = document.createElement('div')
  popup.className = 'seven-days-map__popup'
  const name = document.createElement('strong')
  name.textContent = player.name
  const state = document.createElement('span')
  state.textContent = player.online ? 'Online now' : 'Last known position'
  const coordinates = document.createElement('span')
  coordinates.textContent = coordinateText(
    player.position?.x ?? 0,
    player.position?.y ?? 0,
    player.position?.z ?? 0,
  )
  popup.append(name, state, coordinates)
  if (!player.online && player.lastSeenAt !== undefined) {
    const lastSeen = document.createElement('span')
    lastSeen.textContent = `Seen ${timestampDate(player.lastSeenAt).toLocaleString()}`
    popup.append(lastSeen)
  }
  return popup
}

function createPlayerLabel(player: SevenDaysToDieMapPlayer): HTMLElement {
  const label = document.createElement('span')
  label.textContent = player.name
  return label
}

function syncPlayers(): void {
  if (map === null || playerLayer === null) {
    return
  }

  playerLayer.clearLayers()
  for (const player of players.value) {
    const position = player.position
    if (position === undefined) {
      continue
    }
    const playerState = player.online ? 'online' : 'offline'
    const icon = L.divIcon({
      className: 'seven-days-map__marker-shell',
      html: `<span class="seven-days-map__player seven-days-map__player--${playerState}"><span class="material-icons" aria-hidden="true">person</span></span>`,
      iconSize: [30, 30],
      iconAnchor: [15, 15],
      popupAnchor: [0, -16],
    })
    L.marker([position.x, position.z], { icon, title: player.name })
      .bindTooltip(createPlayerLabel(player), {
        className: `seven-days-map__player-name seven-days-map__player-name--${playerState}`,
        direction: 'right',
        offset: L.point(17, 0),
        permanent: true,
      })
      .bindPopup(createPlayerPopup(player))
      .addTo(playerLayer)
  }
}

function teardownMap(): void {
  resizeObserver?.disconnect()
  resizeObserver = null
  tileLayer = null
  playerLayer = null
  map?.remove()
  map = null
  initializedKey = ''
}

async function initializeMap(): Promise<void> {
  const view = props.view
  const mapSize = view?.mapSize
  if (
    view === null ||
    !view.enabled ||
    mapSize === undefined ||
    view.tileSize <= 0 ||
    view.tileUrlTemplate === ''
  ) {
    teardownMap()
    return
  }

  const key = [
    view.gameServerId,
    view.tileSize,
    view.maxZoom,
    mapSize.x,
    mapSize.z,
    view.tileUrlTemplate,
    props.shareToken,
  ].join(':')
  if (map !== null && initializedKey === key) {
    syncPlayers()
    return
  }

  teardownMap()
  await nextTick()
  if (mapElement.value === null) {
    return
  }

  const minimumZoom = Math.max(0, view.maxZoom - 5)
  const worldBounds = L.latLngBounds(
    [-mapSize.x / 2, -mapSize.z / 2],
    [mapSize.x / 2, mapSize.z / 2],
  )
  map = L.map(mapElement.value, {
    attributionControl: false,
    crs: createSevenDaysToDieCRS(view.maxZoom),
    maxBounds: worldBounds.pad(0.2),
    maxBoundsViscosity: 0.75,
    maxZoom: view.maxZoom + 1,
    minZoom: minimumZoom,
    zoomControl: true,
  })
  tileLayer = new AuthorizedTileLayer(view.tileUrlTemplate, props.shareToken, {
    tileSize: view.tileSize,
    minNativeZoom: 0,
    maxNativeZoom: view.maxZoom,
    minZoom: minimumZoom,
    maxZoom: view.maxZoom + 1,
    noWrap: true,
    updateWhenIdle: false,
  })
  tileLayer.addTo(map)
  lastFullTileRefreshAt = Date.now()
  playerLayer = L.layerGroup().addTo(map)
  const initialView = initialSevenDaysToDieMapView(view.maxZoom, players.value)
  map.setView(initialView.center, initialView.zoom, { animate: false })
  initializedKey = key
  syncPlayers()

  resizeObserver = new ResizeObserver(() => map?.invalidateSize({ pan: false }))
  resizeObserver.observe(mapElement.value)
}

function refresh(): void {
  tileLayer?.redraw()
  emit('refresh')
}

watch(
  () => [
    props.view?.gameServerId,
    props.view?.enabled,
    props.view?.tileSize,
    props.view?.maxZoom,
    props.view?.mapSize?.x,
    props.view?.mapSize?.z,
    props.view?.tileUrlTemplate,
    props.shareToken,
  ],
  () => void initializeMap(),
  { immediate: true },
)
watch(
  () => props.view?.collectedAt?.seconds,
  () => {
    const currentTileLayer = tileLayer
    if (map === null || currentTileLayer === null) {
      return
    }
    const now = Date.now()
    if (now - lastFullTileRefreshAt >= fullTileRefreshIntervalMilliseconds) {
      currentTileLayer.refreshAllTiles()
      lastFullTileRefreshAt = now
      return
    }
    currentTileLayer.refreshPlayerTiles(map, players.value)
  },
)
watch(players, syncPlayers, { deep: true })
onBeforeUnmount(teardownMap)
</script>

<template>
  <section class="seven-days-map" data-testid="seven-days-map">
    <div class="seven-days-map__toolbar">
      <div class="seven-days-map__live-state">
        <span class="seven-days-map__status" :data-tone="mapStatus.tone">
          <q-icon :name="mapStatus.icon" />
          {{ mapStatus.label }}
        </span>
        <span class="seven-days-map__updated">{{ collectedLabel }}</span>
      </div>
      <div class="seven-days-map__summary">
        <span
          ><strong>{{ onlinePlayers }}</strong> online</span
        >
      </div>
      <div class="seven-days-map__toolbar-actions">
        <q-btn aria-label="Refresh live map" dense flat icon="refresh" round @click="refresh">
          <q-tooltip>Refresh map</q-tooltip>
        </q-btn>
      </div>
    </div>

    <div class="seven-days-map__viewport-shell">
      <div
        ref="mapElement"
        aria-label="7 Days to Die world map"
        class="seven-days-map__viewport"
        role="region"></div>

      <div v-if="loading && view === null" class="seven-days-map__overlay">
        <q-spinner color="primary" size="42px" />
        <strong>Loading the world...</strong>
      </div>
      <div v-else-if="view === null || !view.enabled" class="seven-days-map__overlay">
        <q-icon name="map" size="48px" />
        <strong>Native map rendering is unavailable</strong>
        <span>{{
          view?.statusMessage || 'Start the server with Web Dashboard and map rendering enabled.'
        }}</span>
        <q-btn v-if="loadError" color="primary" label="Try again" no-caps @click="refresh" />
      </div>

      <div v-if="view?.stale && view.enabled" class="seven-days-map__stale-banner">
        <q-icon name="history" />
        {{
          view.statusMessage ||
          'Showing the latest cached positions while the server is unavailable.'
        }}
      </div>
    </div>
  </section>
</template>

<style scoped>
.seven-days-map {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 520px;
  overflow: hidden;
  color: var(--xy-text-primary);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.seven-days-map__toolbar {
  z-index: 2;
  display: flex;
  align-items: center;
  gap: var(--xy-space-lg);
  min-height: 60px;
  padding: var(--xy-space-base) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.seven-days-map__live-state {
  display: grid;
  gap: var(--xy-space-2xs);
}

.seven-days-map__status {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-sm);
}

.seven-days-map__status[data-tone='success'] {
  color: var(--xy-success);
}

.seven-days-map__status[data-tone='warning'] {
  color: var(--xy-warning);
}

.seven-days-map__status[data-tone='danger'] {
  color: var(--xy-danger);
}

.seven-days-map__status[data-tone='info'] {
  color: var(--xy-info);
}

.seven-days-map__updated {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.seven-days-map__summary {
  display: flex;
  gap: var(--xy-space-lg);
  margin-left: auto;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.seven-days-map__summary strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.seven-days-map__toolbar-actions {
  display: flex;
  gap: var(--xy-space-xs);
}

.seven-days-map__viewport-shell {
  position: relative;
  flex: 1;
  min-height: 450px;
  overflow: hidden;
  background: var(--xy-base);
}

.seven-days-map__viewport {
  width: 100%;
  height: 100%;
  min-height: 450px;
  background-color: var(--xy-base);
}

.seven-days-map__overlay {
  position: absolute;
  z-index: 600;
  inset: 0;
  display: grid;
  place-items: center;
  align-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
  background: var(--xy-base);
}

.seven-days-map__overlay strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.seven-days-map__overlay span {
  max-width: 52ch;
}

.seven-days-map__stale-banner {
  position: absolute;
  z-index: 510;
  right: var(--xy-space-md);
  bottom: var(--xy-space-md);
  left: var(--xy-space-md);
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  max-width: max-content;
  padding: var(--xy-space-sm) var(--xy-space-base);
  color: var(--xy-warning);
  font-size: var(--xy-font-size-sm);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-lg);
}

:deep(.leaflet-control-zoom a) {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  border-color: var(--xy-border);
}

:deep(.leaflet-control-zoom a:hover) {
  background: var(--xy-surface-3);
}

:deep(.leaflet-popup-content-wrapper),
:deep(.leaflet-popup-tip) {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
}

:deep(.leaflet-popup-content) {
  margin: var(--xy-space-base);
}

:deep(.seven-days-map__popup) {
  display: grid;
  gap: var(--xy-space-xs);
  min-width: 180px;
  color: var(--xy-text-secondary);
}

:deep(.seven-days-map__popup strong) {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
}

:deep(.seven-days-map__marker-shell) {
  background: transparent;
  border: 0;
}

:deep(.seven-days-map__player) {
  position: relative;
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  color: var(--xy-accent-hover);
  background: var(--xy-surface-1);
  border: 2px solid var(--xy-accent);
  border-radius: 50%;
  box-shadow: var(--xy-shadow-md);
  outline: 3px solid var(--xy-accent-bg);
}

:deep(.seven-days-map__player .material-icons) {
  font-size: 19px;
}

:deep(.seven-days-map__player--offline) {
  filter: saturate(0.2);
  opacity: 0.66;
}

:deep(.seven-days-map__player--online::after) {
  position: absolute;
  inset: -6px;
  content: '';
  border: 1px solid var(--xy-accent-border);
  border-radius: 50%;
}

:deep(.seven-days-map__player-name) {
  max-width: 180px;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-sm);
  box-shadow: var(--xy-shadow-md);
}

:deep(.seven-days-map__player-name::before) {
  display: none;
}

:deep(.seven-days-map__player-name--offline) {
  color: var(--xy-text-secondary);
}

@media (max-width: 700px) {
  .seven-days-map {
    min-height: 480px;
    border-right: 0;
    border-left: 0;
    border-radius: 0;
  }

  .seven-days-map__toolbar {
    flex-wrap: wrap;
    gap: var(--xy-space-sm);
  }

  .seven-days-map__summary {
    order: 3;
    width: 100%;
    margin: 0;
  }

  .seven-days-map__toolbar-actions {
    margin-left: auto;
  }
}
</style>
