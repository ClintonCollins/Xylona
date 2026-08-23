<script lang="ts" setup>
import { timestampDate } from '@bufbuild/protobuf/wkt'
import L, { type DoneCallback, type Map as LeafletMap } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import animalIconURL from '@/assets/seven-days-to-die-map/animal.webp'
import bearIconURL from '@/assets/seven-days-to-die-map/bear.webp'
import beeSwarmIconURL from '@/assets/seven-days-to-die-map/bee-swarm.webp'
import boarIconURL from '@/assets/seven-days-to-die-map/boar.webp'
import chickenIconURL from '@/assets/seven-days-to-die-map/chicken.webp'
import coyoteIconURL from '@/assets/seven-days-to-die-map/coyote.webp'
import doeIconURL from '@/assets/seven-days-to-die-map/doe.webp'
import insectSwarmIconURL from '@/assets/seven-days-to-die-map/insect-swarm.webp'
import littleBearIconURL from '@/assets/seven-days-to-die-map/little-bear.webp'
import mountainLionIconURL from '@/assets/seven-days-to-die-map/mountain-lion.webp'
import rabbitIconURL from '@/assets/seven-days-to-die-map/rabbit.webp'
import rattlesnakeIconURL from '@/assets/seven-days-to-die-map/rattlesnake.webp'
import stagIconURL from '@/assets/seven-days-to-die-map/stag.webp'
import wolfIconURL from '@/assets/seven-days-to-die-map/wolf.webp'
import zombieIconURL from '@/assets/seven-days-to-die-map/zombie.webp'
import {
  type SevenDaysToDieMapPlayer,
  type SevenDaysToDieMapView,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import {
  formatSevenDaysToDieCoordinate,
  initialSevenDaysToDieMapView,
  sevenDaysToDieTileURL,
} from '@/pages/game_servers/seven-days-to-die-map'

const fullTileRefreshIntervalMilliseconds = 30_000
const animalIcons = [
  { keywords: ['littlebear', 'bearsmall', 'smallbear', 'bearcub'], url: littleBearIconURL },
  { keywords: ['mountainlion'], url: mountainLionIconURL },
  { keywords: ['beeswarm'], url: beeSwarmIconURL },
  { keywords: ['insectswarm'], url: insectSwarmIconURL },
  { keywords: ['rattlesnake', 'snake'], url: rattlesnakeIconURL },
  { keywords: ['chicken'], url: chickenIconURL },
  { keywords: ['coyote'], url: coyoteIconURL },
  { keywords: ['rabbit'], url: rabbitIconURL },
  { keywords: ['stag'], url: stagIconURL },
  { keywords: ['doe'], url: doeIconURL },
  { keywords: ['boar'], url: boarIconURL },
  { keywords: ['bear'], url: bearIconURL },
  { keywords: ['wolf'], url: wolfIconURL },
] as const
const zombieKeywords = ['zombie', 'zomie', 'direwolf', 'grace', 'vulture'] as const

const props = withDefaults(
  defineProps<{
    view: SevenDaysToDieMapView | null
    loading?: boolean
    loadError?: boolean
    refreshing?: boolean
    publicIdentifier?: string
    configurationPath?: string
  }>(),
  {
    loading: false,
    loadError: false,
    refreshing: false,
    publicIdentifier: '',
    configurationPath: '',
  },
)

const emit = defineEmits<{
  refresh: []
}>()

const mapRoot = ref<HTMLElement | null>(null)
const mapElement = ref<HTMLElement | null>(null)
const fullscreen = ref(false)
const followedPlayerID = ref<string | null>(null)

let map: LeafletMap | null = null
let tileLayer: AuthorizedTileLayer | null = null
let playerLayer: L.LayerGroup | null = null
let overlayLayer: L.LayerGroup | null = null
let resizeObserver: ResizeObserver | null = null
let initializedKey = ''
let lastFullTileRefreshAt = Date.now()

const players = computed(() => props.view?.players ?? [])
const followedPlayer = computed(() =>
  players.value.find(
    (player) => player.id === followedPlayerID.value && player.position !== undefined,
  ),
)
const mapStructureKey = computed(() =>
  [
    props.view?.gameServerId,
    props.view?.enabled,
    props.view?.tileSize,
    props.view?.maxZoom,
    props.view?.mapSize?.x,
    props.view?.mapSize?.z,
    props.view?.tileUrlTemplate,
    props.publicIdentifier,
  ].join(':'),
)
const showClaims = ref(true)
const showHostiles = ref(true)
const showAnimals = ref(true)
const availableState =
  SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
const unspecifiedState =
  SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED
const playersWithPositions = computed(() =>
  players.value.filter((player) => player.position !== undefined),
)
const onlinePlayers = computed(
  () => playersWithPositions.value.filter((player) => player.online).length,
)
const lastKnownPlayers = computed(
  () => playersWithPositions.value.filter((player) => !player.online).length,
)
const unavailableTacticalLayers = computed(() =>
  [
    { label: 'Land claims', state: props.view?.claimsState },
    { label: 'Blood Moon', state: props.view?.bloodMoonState },
    { label: 'Hostiles', state: props.view?.hostileState },
    { label: 'Animals', state: props.view?.animalState },
  ].flatMap((layer) =>
    layer.state !== undefined && layer.state !== unspecifiedState && layer.state !== availableState
      ? [{ label: layer.label, state: overlayStateLabel(layer.state) }]
      : [],
  ),
)
const hasAvailableTacticalLayers = computed(
  () =>
    !props.loadError &&
    [props.view?.claimsState, props.view?.hostileState, props.view?.animalState].some(
      (state) => state === availableState,
    ),
)
const mapDataSummaryLabel = computed(() =>
  unavailableTacticalLayers.value.length > 0 ? 'Map key & data' : 'Map key',
)
const bloodMoonActive = computed(
  () =>
    !props.loadError &&
    props.view?.bloodMoonState === availableState &&
    props.view.bloodMoon?.active === true,
)
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
  private readonly publicIdentifier: string

  constructor(template: string, publicIdentifier: string, options: L.GridLayerOptions) {
    super(options)
    this.template = template
    this.publicIdentifier = publicIdentifier
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
    if (this.publicIdentifier !== '') {
      headers['X-Xylona-Map-Share'] = this.publicIdentifier
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

function overlayStateLabel(state: SevenDaysToDieWebAPIValueState): string {
  switch (state) {
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE:
      return 'Available'
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED:
      return 'Unsupported'
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED:
      return 'Upstream access denied'
    default:
      return 'Unavailable'
  }
}

function createMapPopup(name: string, x: number, y: number, z: number): HTMLElement {
  const popup = document.createElement('div')
  popup.className = 'seven-days-map__popup'
  const heading = document.createElement('strong')
  heading.textContent = name
  const coordinates = document.createElement('span')
  coordinates.textContent = coordinateText(x, y, z)
  popup.append(heading, coordinates)
  return popup
}

function createMapLabel(name: string): HTMLElement {
  const label = document.createElement('span')
  label.textContent = name
  return label
}

function createEntityIcon(name: string, hostile: boolean): L.DivIcon {
  const normalizedName = name.toLowerCase().replaceAll(/[^a-z]/g, '')
  const isZombie = hostile && zombieKeywords.some((keyword) => normalizedName.includes(keyword))
  const iconURL = isZombie
    ? zombieIconURL
    : (animalIcons.find(({ keywords }) =>
        keywords.some((keyword) => normalizedName.includes(keyword)),
      )?.url ?? (hostile ? zombieIconURL : animalIconURL))
  const tone = hostile ? 'hostile' : 'animal'

  return L.divIcon({
    className: 'seven-days-map__marker-shell',
    html: `<span class="seven-days-map__entity seven-days-map__entity--${tone}"><img src="${iconURL}" alt="" aria-hidden="true"></span>`,
    iconSize: [30, 30],
    iconAnchor: [15, 15],
    popupAnchor: [0, -16],
  })
}

function syncOverlays(): void {
  if (map === null || overlayLayer === null) {
    return
  }

  overlayLayer.clearLayers()
  for (const note of props.view?.markers ?? []) {
    const icon = L.divIcon({
      className: 'seven-days-map__marker-shell',
      html: '<span class="seven-days-map__map-pin seven-days-map__map-pin--note"><span class="material-icons" aria-hidden="true">edit_location</span></span>',
      iconSize: [30, 30],
      iconAnchor: [15, 15],
    })
    L.marker([note.x, note.z], { icon, title: note.name })
      .bindTooltip(createMapLabel(note.name))
      .bindPopup(createMapPopup(note.name, note.x, 0, note.z))
      .addTo(overlayLayer)
  }

  if (props.loadError) return

  if (props.view?.claimsState === availableState && showClaims.value) {
    for (const claim of props.view.claims) {
      const position = claim.position
      if (position === undefined) continue
      const radius = claim.size / 2
      L.rectangle(
        [
          [position.x - radius, position.z - radius],
          [position.x + radius, position.z + radius],
        ],
        { color: 'var(--xy-warning)', fillOpacity: 0.12, weight: 2 },
      )
        .bindPopup(
          createMapPopup(
            claim.ownerName || 'Unidentified land claim',
            position.x,
            position.y,
            position.z,
          ),
        )
        .addTo(overlayLayer)
    }
  }

  if (props.view?.hostileState === availableState && showHostiles.value) {
    for (const hostile of props.view.hostiles) {
      const position = hostile.position
      if (position === undefined) continue
      L.marker([position.x, position.z], {
        icon: createEntityIcon(hostile.name, true),
        title: hostile.name,
      })
        .bindPopup(createMapPopup(hostile.name, position.x, position.y, position.z))
        .addTo(overlayLayer)
    }
  }

  if (props.view?.animalState === availableState && showAnimals.value) {
    for (const animal of props.view.animals) {
      const position = animal.position
      if (position === undefined) continue
      L.marker([position.x, position.z], {
        icon: createEntityIcon(animal.name, false),
        title: animal.name,
      })
        .bindPopup(createMapPopup(animal.name, position.x, position.y, position.z))
        .addTo(overlayLayer)
    }
  }
}

function gameTimeLabel(time: { day: number; hour: number; minute: number } | undefined): string {
  if (time === undefined) return 'Not reported'
  return `Day ${time.day}, ${String(time.hour).padStart(2, '0')}:${String(time.minute).padStart(2, '0')}`
}

function syncPlayers(): void {
  const followed = followedPlayer.value
  if (followedPlayerID.value !== null && followed === undefined) {
    followedPlayerID.value = null
  }
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
    const marker = L.marker([position.x, position.z], { icon, title: `Follow ${player.name}` })
    marker.on('click', () => followPlayer(player))
    marker
      .bindTooltip(createPlayerLabel(player), {
        className: `seven-days-map__player-name seven-days-map__player-name--${playerState}`,
        direction: 'right',
        offset: L.point(17, 0),
        permanent: true,
      })
      .bindPopup(createPlayerPopup(player))
      .addTo(playerLayer)
  }

  if (followed !== undefined) {
    panToPlayer(followed)
  }
}

function teardownMap(): void {
  resizeObserver?.disconnect()
  resizeObserver = null
  tileLayer = null
  playerLayer = null
  overlayLayer = null
  map?.remove()
  map = null
  initializedKey = ''
  followedPlayerID.value = null
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

  const key = mapStructureKey.value
  if (map !== null && initializedKey === key) {
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
  tileLayer = new AuthorizedTileLayer(view.tileUrlTemplate, props.publicIdentifier, {
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
  overlayLayer = L.layerGroup().addTo(map)
  const initialView = initialSevenDaysToDieMapView(view.maxZoom, players.value)
  map.setView(initialView.center, initialView.zoom, { animate: false })
  initializedKey = key
  syncPlayers()
  syncOverlays()

  resizeObserver = new ResizeObserver(() => map?.invalidateSize({ pan: false }))
  resizeObserver.observe(mapElement.value)
}

function refresh(): void {
  tileLayer?.redraw()
  emit('refresh')
}

function shouldAnimateMap(): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches !== true
}

function fitWorld(): void {
  const mapSize = props.view?.mapSize
  if (map === null || mapSize === undefined) return
  map.fitBounds(
    [
      [-mapSize.x / 2, -mapSize.z / 2],
      [mapSize.x / 2, mapSize.z / 2],
    ],
    { animate: shouldAnimateMap(), padding: [24, 24] },
  )
}

function panToPlayer(player: SevenDaysToDieMapPlayer): void {
  const position = player.position
  if (map === null || position === undefined) return
  map.panTo([position.x, position.z], { animate: shouldAnimateMap() })
}

function followPlayer(player: SevenDaysToDieMapPlayer): void {
  followedPlayerID.value = player.id
  panToPlayer(player)
}

function stopFollowingPlayer(): void {
  followedPlayerID.value = null
}

function focusPlayer(player: SevenDaysToDieMapPlayer): void {
  const position = player.position
  if (map === null || position === undefined) return
  map.setView([position.x, position.z], Math.max(0, props.view?.maxZoom ?? 0), {
    animate: shouldAnimateMap(),
  })
}

async function toggleFullscreen(): Promise<void> {
  if (mapRoot.value === null) return

  try {
    if (document.fullscreenElement === mapRoot.value) {
      await document.exitFullscreen()
    } else {
      await mapRoot.value.requestFullscreen()
    }
  } catch (unknownError: unknown) {
    console.error('Could not change 7 Days to Die map fullscreen state', unknownError)
  }
}

function handleFullscreenChange(): void {
  fullscreen.value = document.fullscreenElement === mapRoot.value
  void nextTick(() => map?.invalidateSize({ pan: false }))
}

watch(mapStructureKey, () => void initializeMap(), { immediate: true })
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
watch(() => props.view, syncOverlays, { deep: true })
watch(() => props.loadError, syncOverlays)
watch([showClaims, showHostiles, showAnimals], syncOverlays)
onMounted(() => document.addEventListener('fullscreenchange', handleFullscreenChange))
onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  teardownMap()
})
</script>

<template>
  <section
    ref="mapRoot"
    class="seven-days-map"
    :class="{ 'seven-days-map--fullscreen': fullscreen }"
    :data-blood-moon-active="bloodMoonActive"
    data-testid="seven-days-map">
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
        <span v-if="lastKnownPlayers > 0"
          ><strong>{{ lastKnownPlayers }}</strong> last known</span
        >
      </div>
      <div class="seven-days-map__toolbar-actions">
        <q-btn
          v-if="followedPlayer"
          :aria-label="`Stop following ${followedPlayer.name}`"
          color="accent"
          data-testid="stop-following-player"
          dense
          flat
          icon="gps_fixed"
          :label="`Following ${followedPlayer.name}`"
          no-caps
          @click="stopFollowingPlayer">
          <q-tooltip>Stop following {{ followedPlayer.name }}</q-tooltip>
        </q-btn>
        <q-btn
          :loading="refreshing"
          :aria-label="refreshing ? 'Scanning world data' : 'Refresh live data'"
          data-testid="refresh-live-data"
          dense
          flat
          icon="refresh"
          :label="refreshing ? 'Scanning world' : 'Refresh'"
          no-caps
          @click="refresh" />
        <q-btn
          :disable="view?.enabled !== true"
          aria-label="Fit the whole world in view"
          data-testid="fit-world"
          dense
          flat
          icon="zoom_out_map"
          label="Fit world"
          no-caps
          @click="fitWorld" />
        <q-btn
          :aria-label="fullscreen ? 'Exit fullscreen map' : 'Open fullscreen map'"
          data-testid="fullscreen-map"
          dense
          flat
          :icon="fullscreen ? 'fullscreen_exit' : 'fullscreen'"
          round
          @click="toggleFullscreen">
          <q-tooltip>{{ fullscreen ? 'Exit fullscreen' : 'Fullscreen map' }}</q-tooltip>
        </q-btn>
        <q-btn-dropdown
          v-if="playersWithPositions.length > 1"
          aria-label="Focus a player on the map"
          dense
          flat
          icon="person_pin_circle"
          label="Players"
          no-caps>
          <q-list dense>
            <q-item
              v-for="(player, index) in playersWithPositions"
              :key="player.id || index"
              v-close-popup
              clickable
              :data-testid="`focus-player-${player.id || index}`"
              @click="focusPlayer(player)">
              <q-item-section avatar>
                <q-icon :color="player.online ? 'positive' : undefined" name="person" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ player.name }}</q-item-label>
                <q-item-label caption>{{ player.online ? 'Online' : 'Last known' }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-btn-dropdown>
      </div>
    </div>

    <div aria-label="Map tools" class="seven-days-map__layer-controls" data-testid="map-tools">
      <div
        v-if="hasAvailableTacticalLayers"
        aria-label="Tactical map layers"
        class="seven-days-map__toggles"
        data-testid="tactical-layer-controls"
        role="group">
        <q-toggle
          v-if="view?.claimsState === availableState"
          v-model="showClaims"
          label="Land claims"
          aria-label="Toggle land claims"
          data-testid="toggle-land-claims" />
        <q-toggle
          v-if="view?.hostileState === availableState"
          v-model="showHostiles"
          label="Hostiles"
          aria-label="Toggle hostile positions"
          data-testid="toggle-hostiles" />
        <q-toggle
          v-if="view?.animalState === availableState"
          v-model="showAnimals"
          label="Animals"
          aria-label="Toggle animal positions"
          data-testid="toggle-animals" />
      </div>

      <span
        v-if="!loadError && view?.bloodMoonState === availableState && view.bloodMoon"
        class="seven-days-map__blood-moon"
        :data-active="view.bloodMoon.active"
        data-testid="blood-moon-status">
        <q-icon name="dark_mode" />
        <strong>{{ view.bloodMoon.active ? 'Blood Moon active' : 'Blood Moon inactive' }}</strong>
        <span>{{ gameTimeLabel(view.bloodMoon.gameTime) }}</span>
      </span>

      <details class="seven-days-map__map-data" data-testid="map-data-disclosure">
        <summary :aria-label="mapDataSummaryLabel">
          <q-icon name="info_outline" />
          <span class="seven-days-map__map-data-label">{{ mapDataSummaryLabel }}</span>
        </summary>
        <div class="seven-days-map__map-data-content">
          <div aria-label="Map legend" class="seven-days-map__legend">
            <strong>Map key</strong>
            <span><q-icon color="accent" name="person" /> Online player</span>
            <span><q-icon name="history" /> Last-known player</span>
            <span><q-icon color="purple" name="edit_location" /> Map note</span>
            <span v-if="!loadError && view?.hostileState === availableState"
              ><q-icon color="negative" name="warning" /> Hostile</span
            >
            <span v-if="!loadError && view?.animalState === availableState"
              ><q-icon color="positive" name="pets" /> Animal</span
            >
          </div>
          <div v-if="unavailableTacticalLayers.length > 0" class="seven-days-map__unavailable-data">
            <strong>Unavailable map data</strong>
            <dl>
              <div v-for="layer in unavailableTacticalLayers" :key="layer.label">
                <dt>{{ layer.label }}</dt>
                <dd>{{ layer.state }}</dd>
              </div>
            </dl>
            <router-link
              v-if="configurationPath"
              class="seven-days-map__configuration-link"
              :to="configurationPath">
              <q-icon name="tune" />
              Open map configuration
            </router-link>
            <span v-else class="seven-days-map__configuration-hint">
              A server administrator can review the Web Dashboard configuration.
            </span>
          </div>
        </div>
      </details>
    </div>

    <div class="seven-days-map__viewport-shell">
      <div
        ref="mapElement"
        aria-label="7 Days to Die world map"
        class="seven-days-map__viewport"
        role="region"></div>

      <div
        v-if="refreshing && view?.enabled"
        aria-hidden="true"
        class="seven-days-map__scan"
        data-testid="world-scan"></div>

      <div v-if="loading && view === null" class="seven-days-map__overlay">
        <q-spinner color="primary" size="42px" />
        <strong>Surveying the world…</strong>
      </div>
      <div v-else-if="view === null || !view.enabled" class="seven-days-map__overlay">
        <q-icon name="map" size="48px" />
        <strong>Native map rendering is unavailable</strong>
        <span>{{
          view?.statusMessage || 'Start the server with Web Dashboard and map rendering enabled.'
        }}</span>
        <div class="seven-days-map__overlay-actions">
          <q-btn v-if="loadError" color="primary" label="Try again" no-caps @click="refresh" />
          <q-btn
            v-if="configurationPath"
            :to="configurationPath"
            flat
            icon="tune"
            label="Open configuration"
            no-caps />
        </div>
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

.seven-days-map--fullscreen {
  width: 100vw;
  height: 100dvh;
  min-height: 0;
  border: 0;
  border-radius: 0;
}

.seven-days-map--fullscreen .seven-days-map__viewport-shell,
.seven-days-map--fullscreen .seven-days-map__viewport {
  min-height: 0;
}

.seven-days-map__toolbar {
  z-index: 2;
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--xy-space-base);
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
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-base);
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
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--xy-space-xs);
}

.seven-days-map__layer-controls {
  position: relative;
  z-index: 2000;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  font-size: var(--xy-font-size-sm);
}

.seven-days-map__toggles {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-md);
}

.seven-days-map__blood-moon {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-xs);
}

.seven-days-map__blood-moon strong {
  color: var(--xy-text-primary);
}

.seven-days-map__blood-moon[data-active='true'] {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
  border-color: var(--xy-danger-border);
}

.seven-days-map__blood-moon[data-active='true'] strong {
  color: var(--xy-danger-hover);
}

.seven-days-map__map-data {
  position: relative;
  margin-left: auto;
}

.seven-days-map__map-data summary {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 32px;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-primary);
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font-weight: 600;
  list-style: none;
}

.seven-days-map__map-data summary:hover,
.seven-days-map__map-data summary:focus-visible {
  background: var(--xy-surface-3);
}

.seven-days-map__map-data summary::-webkit-details-marker {
  display: none;
}

.seven-days-map__map-data-content {
  position: absolute;
  z-index: 2100;
  top: calc(100% + var(--xy-space-xs));
  right: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-lg);
  width: min(560px, calc(100vw - var(--xy-space-xl)));
  max-height: min(60vh, 480px);
  padding: var(--xy-space-md);
  box-sizing: border-box;
  overflow-y: auto;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-lg);
}

.seven-days-map__legend,
.seven-days-map__unavailable-data {
  display: grid;
  align-content: start;
  gap: var(--xy-space-sm);
}

.seven-days-map__legend > span {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.seven-days-map__unavailable-data dl {
  display: grid;
  gap: var(--xy-space-xs);
  margin: 0;
}

.seven-days-map__unavailable-data dl > div {
  display: flex;
  justify-content: space-between;
  gap: var(--xy-space-base);
}

.seven-days-map__unavailable-data dt {
  color: var(--xy-text-primary);
}

.seven-days-map__unavailable-data dd {
  margin: 0;
}

.seven-days-map__configuration-link {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  width: fit-content;
  color: var(--xy-primary-hover);
  font-weight: 600;
  text-decoration: none;
}

.seven-days-map__configuration-link:hover {
  text-decoration: underline;
  text-underline-offset: 0.2em;
}

.seven-days-map__configuration-hint {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
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

.seven-days-map__scan {
  position: absolute;
  z-index: 500;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.seven-days-map__scan::before {
  position: absolute;
  top: -20%;
  right: 0;
  left: 0;
  height: 20%;
  content: '';
  background: linear-gradient(
    to bottom,
    transparent,
    color-mix(in srgb, var(--xy-accent) 12%, transparent),
    var(--xy-accent-border),
    transparent
  );
  opacity: 0.7;
  animation: seven-days-map-scan 1s var(--xy-ease-standard) infinite;
}

.seven-days-map[data-blood-moon-active='true'] .seven-days-map__viewport-shell::after {
  position: absolute;
  z-index: 490;
  inset: 0;
  content: '';
  pointer-events: none;
  box-shadow:
    inset 0 0 0 1px var(--xy-danger-border),
    inset 0 0 var(--xy-space-3xl) color-mix(in srgb, var(--xy-danger) 14%, transparent);
  animation: seven-days-map-blood-moon 900ms var(--xy-ease-standard) both;
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

.seven-days-map__overlay-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--xy-space-sm);
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
  font-size: var(--xy-font-size-lg);
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

:deep(.seven-days-map__map-pin),
:deep(.seven-days-map__entity) {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 2px solid var(--xy-accent);
  border-radius: 50%;
  box-shadow: var(--xy-shadow-md);
}

:deep(.seven-days-map__entity) {
  box-sizing: border-box;
  padding: 3px;
}

:deep(.seven-days-map__entity img) {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

:deep(.seven-days-map__entity--animal) {
  border-color: var(--xy-success);
}

:deep(.seven-days-map__entity--hostile) {
  border-color: var(--xy-danger);
}

:deep(.seven-days-map__map-pin--note) {
  border-color: var(--xy-purple);
}

@keyframes seven-days-map-scan {
  from {
    transform: translateY(0);
  }

  to {
    transform: translateY(600%);
  }
}

@keyframes seven-days-map-blood-moon {
  0% {
    opacity: 0.4;
    box-shadow:
      inset 0 0 0 1px var(--xy-danger-border),
      inset 0 0 var(--xy-space-lg) color-mix(in srgb, var(--xy-danger) 8%, transparent);
  }

  48% {
    opacity: 1;
    box-shadow:
      inset 0 0 0 1px var(--xy-danger-border),
      inset 0 0 var(--xy-space-3xl) color-mix(in srgb, var(--xy-danger) 20%, transparent);
  }

  100% {
    opacity: 0.72;
  }
}

@media (prefers-reduced-motion: reduce) {
  .seven-days-map__scan::before,
  .seven-days-map[data-blood-moon-active='true'] .seven-days-map__viewport-shell::after {
    animation: none;
  }

  .seven-days-map__scan::before {
    top: 40%;
  }
}

@media (min-width: 1200px) {
  .seven-days-map__map-data summary {
    min-width: 36px;
    justify-content: center;
  }

  .seven-days-map__map-data-label {
    display: none;
  }
}

@media (max-width: 700px) {
  .seven-days-map {
    min-height: 480px;
    border-right: 0;
    border-left: 0;
    border-radius: 0;
  }

  .seven-days-map__toolbar {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: var(--xy-space-sm);
  }

  .seven-days-map__summary {
    margin: 0;
  }

  .seven-days-map__toolbar-actions {
    grid-column: 1 / -1;
    justify-content: flex-start;
  }

  .seven-days-map__layer-controls {
    gap: var(--xy-space-xs) var(--xy-space-base);
  }

  .seven-days-map__map-data-content {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 599px) {
  .seven-days-map__map-data summary {
    min-width: 36px;
    justify-content: center;
  }

  .seven-days-map__map-data-label {
    display: none;
  }
}
</style>
