import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieMapPlayerSchema,
  SevenDaysToDieMapVectorSchema,
  SevenDaysToDieMapViewSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import SevenDaysToDieLiveMap from './SevenDaysToDieLiveMap.vue'

const leafletMocks = vi.hoisted(() => ({
  layers: [] as object[],
  layerGroups: [] as { clearLayers: ReturnType<typeof vi.fn> }[],
  overlays: [] as {
    events?: Record<string, () => void>
    group?: object
    icon?: { html?: string }
    kind: string
    popup?: HTMLElement
    title?: string
  }[],
  fitBounds: vi.fn(),
  panTo: vi.fn(),
  redraw: vi.fn(),
  setView: vi.fn(),
}))

vi.mock('leaflet', () => {
  class GridLayer {
    constructor() {
      leafletMocks.layers.push(this)
    }

    addTo(): this {
      return this
    }

    getTileSize(): { x: number; y: number } {
      return { x: 128, y: 128 }
    }

    on(): this {
      return this
    }

    redraw(): this {
      leafletMocks.redraw()
      return this
    }
  }

  const bounds = { pad: vi.fn(() => bounds) }
  const map = {
    fitBounds: leafletMocks.fitBounds,
    invalidateSize: vi.fn(),
    panTo: leafletMocks.panTo,
    project: vi.fn(() => ({
      unscaleBy: vi.fn(() => ({
        floor: vi.fn(() => ({ x: 1, y: -2 })),
      })),
    })),
    remove: vi.fn(),
    setView: leafletMocks.setView,
  }
  function overlay(kind: string, options?: { icon?: { html?: string }; title?: string }) {
    const record: {
      events?: Record<string, () => void>
      group?: object
      icon?: { html?: string }
      kind: string
      popup?: HTMLElement
      title?: string
    } = {
      icon: options?.icon,
      kind,
      title: options?.title,
    }
    leafletMocks.overlays.push(record)
    const result = {
      addTo(group: object) {
        record.group = group
        return result
      },
      bindPopup(popup: HTMLElement) {
        record.popup = popup
        return result
      },
      bindTooltip() {
        return result
      },
      on(event: string, handler: () => void) {
        record.events ??= {}
        record.events[event] = handler
        return result
      },
    }
    return result
  }

  return {
    default: {
      CRS: { Simple: {} },
      GridLayer,
      Transformation: class {},
      bounds: vi.fn(() => bounds),
      divIcon: vi.fn((options: { html?: string }) => options),
      extend: Object.assign,
      latLngBounds: vi.fn(() => bounds),
      layerGroup: vi.fn(() => {
        const group = { addTo: vi.fn(() => group), clearLayers: vi.fn() }
        leafletMocks.layerGroups.push(group)
        return group
      }),
      map: vi.fn(() => map),
      marker: vi.fn((_position, options) => overlay('marker', options)),
      point: vi.fn(),
      rectangle: vi.fn(() => overlay('claim')),
    },
  }
})

function mapView(collectedAt: Date) {
  return create(SevenDaysToDieMapViewSchema, {
    collectedAt: timestampFromDate(collectedAt),
    enabled: true,
    gameServerId: 'server-1',
    mapSize: create(SevenDaysToDieMapVectorSchema, { x: 6144, z: 6144 }),
    maxZoom: 4,
    players: [
      create(SevenDaysToDieMapPlayerSchema, {
        id: 'player-1',
        name: 'Alex',
        online: true,
        position: create(SevenDaysToDieMapVectorSchema, { x: 130, z: 130 }),
      }),
    ],
    tileSize: 128,
    tileUrlTemplate: '/tiles/{z}/{x}/{y}.png',
  })
}

describe('SevenDaysToDieLiveMap', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-19T12:00:00Z'))
    leafletMocks.layers.length = 0
    leafletMocks.layerGroups.length = 0
    leafletMocks.overlays.length = 0
    leafletMocks.fitBounds.mockClear()
    leafletMocks.panTo.mockClear()
    leafletMocks.redraw.mockClear()
    leafletMocks.setView.mockClear()
    vi.stubGlobal(
      'ResizeObserver',
      class {
        disconnect(): void {}
        observe(): void {}
      },
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => undefined)),
    )
  })

  afterEach(() => {
    Reflect.deleteProperty(document, 'exitFullscreen')
    Reflect.deleteProperty(document, 'fullscreenElement')
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('refreshes map tiles without rebuilding the visible grid', async () => {
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: {
        publicIdentifier: 'Public_Map',
        view: mapView(new Date('2026-08-19T12:00:00Z')),
      },
    })
    await flushPromises()

    const tileLayer = leafletMocks.layers[0] as {
      _tiles: Record<
        string,
        { coords: { x: number; y: number; z: number }; current: boolean; el: HTMLElement }
      >
      createTile(coordinates: { x: number; y: number; z: number }, done: () => void): HTMLElement
    }
    const playerTileCoordinates = { x: 1, y: -2, z: 4 }
    const neighborTileCoordinates = { x: 2, y: -2, z: 4 }
    const farTileCoordinates = { x: 5, y: -2, z: 4 }
    const bufferedTileCoordinates = { x: 1, y: -1, z: 4 }
    const playerTile = tileLayer.createTile(playerTileCoordinates, vi.fn())
    const neighborTile = tileLayer.createTile(neighborTileCoordinates, vi.fn())
    const farTile = tileLayer.createTile(farTileCoordinates, vi.fn())
    const bufferedTile = tileLayer.createTile(bufferedTileCoordinates, vi.fn())
    const tileRequest = vi.mocked(fetch).mock.calls.find(([, options]) => options?.headers)
    expect((tileRequest?.[1]?.headers as Record<string, string>)['X-Xylona-Map-Share']).toBe(
      'Public_Map',
    )
    tileLayer._tiles = {
      '1:-2:4': { coords: playerTileCoordinates, current: true, el: playerTile },
      '2:-2:4': { coords: neighborTileCoordinates, current: true, el: neighborTile },
      '5:-2:4': { coords: farTileCoordinates, current: true, el: farTile },
      '1:-1:4': { coords: bufferedTileCoordinates, current: false, el: bufferedTile },
    }

    vi.setSystemTime(new Date('2026-08-19T12:00:05Z'))
    await wrapper.setProps({ view: mapView(new Date('2026-08-19T12:00:05Z')) })
    await flushPromises()

    expect(leafletMocks.redraw).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(6)
    expect(
      vi
        .mocked(fetch)
        .mock.calls.slice(4)
        .map(([url]) => url),
    ).toEqual(['/tiles/4/1/1.png', '/tiles/4/2/1.png'])

    await wrapper.setProps({ view: mapView(new Date('2026-08-19T12:00:05Z')) })
    await flushPromises()
    expect(leafletMocks.redraw).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(6)

    vi.setSystemTime(new Date('2026-08-19T12:00:30Z'))
    await wrapper.setProps({ view: mapView(new Date('2026-08-19T12:00:30Z')) })
    await flushPromises()
    expect(
      vi
        .mocked(fetch)
        .mock.calls.slice(6)
        .map(([url]) => url),
    ).toEqual(['/tiles/4/1/1.png', '/tiles/4/2/1.png', '/tiles/4/5/1.png'])
    expect(leafletMocks.redraw).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps tactical controls independent on one shared Leaflet layer', async () => {
    const tacticalView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animals: [{ name: 'Wolf', position: { x: 20, y: 3, z: 30 } }],
      bloodMoon: {
        active: true,
        gameTime: { day: 7, hour: 22 },
        nextBloodMoon: { day: 14, hour: 22 },
        nextBloodMoonEnd: { day: 15, hour: 4 },
      },
      bloodMoonState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      claims: [
        {
          active: true,
          ownerName: 'Owner',
          position: { x: 40, y: 5, z: 50 },
          size: 41,
        },
      ],
      claimsState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostiles: [{ name: 'Zombie', position: { x: 60, y: 6, z: 70 } }],
      nativeMarkerState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      nativeMarkers: [{ id: 'marker-1', name: '<img src=x onerror=alert(1)>', x: 10, z: 12 }],
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: { refreshing: true, view: tacticalView },
    })
    await flushPromises()

    expect(wrapper.attributes('data-blood-moon-active')).toBe('true')
    expect(wrapper.get('[data-testid="world-scan"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="refresh-live-data"]').attributes('label')).toBe(
      'Scanning world',
    )
    expect(wrapper.get('[data-testid="tactical-layer-controls"]').attributes('role')).toBe('group')
    expect(wrapper.findAll('[data-testid^="toggle-"]')).toHaveLength(3)
    expect(wrapper.get('[data-testid="blood-moon-status"]').text()).toContain('Blood Moon active')
    expect(leafletMocks.layerGroups).toHaveLength(2)
    expect(leafletMocks.overlays.some((entry) => entry.kind === 'claim')).toBe(true)
    expect(
      leafletMocks.overlays.filter(
        (entry) =>
          entry.popup?.textContent?.includes('Wolf') ||
          entry.popup?.textContent?.includes('Zombie'),
      ),
    ).toHaveLength(2)
    const overlayLayer = leafletMocks.layerGroups[1]
    expect(leafletMocks.overlays.filter((entry) => entry.group === overlayLayer)).toHaveLength(3)
    const unsafeMarker = leafletMocks.overlays.find(
      (entry) => entry.title === '<img src=x onerror=alert(1)>',
    )
    expect(unsafeMarker).toBeUndefined()

    const layerToggleCases = ['toggle-land-claims', 'toggle-hostiles', 'toggle-animals']
    for (const controlTestId of layerToggleCases) {
      const overlayCountBeforeToggle = leafletMocks.overlays.length
      const control = wrapper.getComponent(`[data-testid="${controlTestId}"]`)
      control.vm.$emit('update:modelValue', false)
      await flushPromises()
      const rebuiltOverlays = leafletMocks.overlays.slice(overlayCountBeforeToggle)
      expect(rebuiltOverlays).toHaveLength(2)
      expect(rebuiltOverlays.every((entry) => entry.group === overlayLayer)).toBe(true)
      expect(
        rebuiltOverlays.some((entry) => {
          switch (controlTestId) {
            case 'toggle-land-claims':
              return entry.kind === 'claim'
            case 'toggle-hostiles':
              return entry.popup?.textContent?.includes('Zombie')
            default:
              return entry.popup?.textContent?.includes('Wolf')
          }
        }),
      ).toBe(false)
      control.vm.$emit('update:modelValue', true)
      await flushPromises()
    }

    wrapper.unmount()
  })

  it('fits the world, focuses players, and distinguishes last-known positions', async () => {
    const focusView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      players: [
        create(SevenDaysToDieMapPlayerSchema, {
          id: 'player-1',
          name: 'Alex',
          online: true,
          position: { x: 10, z: 20 },
        }),
        create(SevenDaysToDieMapPlayerSchema, {
          id: 'player-2',
          name: 'Blake',
          online: false,
          position: { x: 30, z: 40 },
        }),
      ],
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      global: { renderStubDefaultSlot: true },
      props: { view: focusView },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('1 online')
    expect(wrapper.text()).toContain('1 last known')

    await wrapper.get('[data-testid="fit-world"]').trigger('click')
    expect(leafletMocks.fitBounds).toHaveBeenCalledWith(
      [
        [-3072, -3072],
        [3072, 3072],
      ],
      { animate: true, padding: [24, 24] },
    )

    await wrapper.get('[data-testid="focus-player-player-2"]').trigger('click')
    expect(leafletMocks.setView).toHaveBeenLastCalledWith([30, 40], 4, { animate: true })

    const alexMarker = leafletMocks.overlays.find((entry) => entry.title === 'Follow Alex')
    expect(alexMarker?.events?.click).toBeTypeOf('function')
    alexMarker?.events?.click?.()
    await wrapper.vm.$nextTick()
    expect(leafletMocks.panTo).toHaveBeenLastCalledWith([10, 20], { animate: true })
    expect(wrapper.get('[data-testid="stop-following-player"]').attributes('label')).toBe(
      'Following Alex',
    )

    const movedView = create(SevenDaysToDieMapViewSchema, {
      ...focusView,
      players: [
        create(SevenDaysToDieMapPlayerSchema, {
          id: 'player-1',
          name: 'Alex',
          online: true,
          position: { x: 50, z: 60 },
        }),
        focusView.players[1],
      ],
    })
    await wrapper.setProps({ view: movedView })
    await flushPromises()
    expect(leafletMocks.panTo).toHaveBeenLastCalledWith([50, 60], { animate: true })

    await wrapper.get('[data-testid="stop-following-player"]').trigger('click')
    expect(wrapper.find('[data-testid="stop-following-player"]').exists()).toBe(false)

    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    await wrapper.get('[data-testid="focus-player-player-2"]').trigger('click')
    expect(leafletMocks.setView).toHaveBeenLastCalledWith([30, 40], 4, { animate: false })
    wrapper.unmount()
  })

  it('opens and exits the native fullscreen map', async () => {
    let fullscreenElement: Element | null = null
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      get: () => fullscreenElement,
    })

    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: { view: mapView(new Date('2026-08-19T12:00:00Z')) },
    })
    const requestFullscreen = vi.fn(async () => {
      fullscreenElement = wrapper.element
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    const exitFullscreen = vi.fn(async () => {
      fullscreenElement = null
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    Object.defineProperty(wrapper.element, 'requestFullscreen', { value: requestFullscreen })
    Object.defineProperty(document, 'exitFullscreen', {
      configurable: true,
      value: exitFullscreen,
    })

    const fullscreenButton = wrapper.get('[data-testid="fullscreen-map"]')
    await fullscreenButton.trigger('click')
    await flushPromises()
    expect(requestFullscreen).toHaveBeenCalledOnce()
    expect(wrapper.classes()).toContain('seven-days-map--fullscreen')
    expect(fullscreenButton.attributes('aria-label')).toBe('Exit fullscreen map')

    await fullscreenButton.trigger('click')
    await flushPromises()
    expect(exitFullscreen).toHaveBeenCalledOnce()
    expect(wrapper.classes()).not.toContain('seven-days-map--fullscreen')
    wrapper.unmount()
  })

  it('uses species icons for animals and one fallback icon for zombies', async () => {
    const species = [
      ['Bee Swarm', 'bee-swarm.webp'],
      ['Boar', 'boar.webp'],
      ['Chicken', 'chicken.webp'],
      ['Coyote', 'coyote.webp'],
      ['Doe', 'doe.webp'],
      ['Insect Swarm', 'insect-swarm.webp'],
      ['animalBearSmall', 'little-bear.webp'],
      ['Mountain Lion', 'mountain-lion.webp'],
      ['Rabbit', 'rabbit.webp'],
      ['Rattlesnake', 'rattlesnake.webp'],
      ['Stag', 'stag.webp'],
      ['Wolf', 'wolf.webp'],
      ['Bear', 'bear.webp'],
    ] as const
    const iconView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animals: [
        ...species.map(([name], index) => ({ name, position: { x: index + 1, z: index + 2 } })),
        { name: 'Modded alpaca', position: { x: 20, z: 21 } },
      ],
      hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostiles: [
        { name: 'animalMountainLion', position: { x: 30, z: 31 } },
        { name: 'Zombie Arlene', position: { x: 32, z: 33 } },
        { name: 'animalZomieBear', position: { x: 34, z: 35 } },
      ],
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, { props: { view: iconView } })
    await flushPromises()

    const markerHTML = (name: string) =>
      leafletMocks.overlays.find((entry) => entry.title === name)?.icon?.html
    for (const [name, fileName] of species) {
      expect(markerHTML(name)).toContain(fileName)
    }
    expect(markerHTML('Modded alpaca')).toContain('animal.webp')
    expect(markerHTML('animalMountainLion')).toContain('mountain-lion.webp')
    expect(markerHTML('Zombie Arlene')).toContain('zombie.webp')
    expect(markerHTML('animalZomieBear')).toContain('zombie.webp')
    wrapper.unmount()
  })

  it('rebuilds each Leaflet data layer once for a same-map snapshot', async () => {
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: { view: mapView(new Date('2026-08-19T12:00:00Z')) },
    })
    await flushPromises()

    expect(leafletMocks.layerGroups).toHaveLength(2)
    for (const group of leafletMocks.layerGroups) {
      expect(group.clearLayers).toHaveBeenCalledTimes(1)
    }
    const tileLayer = leafletMocks.layers[0] as { _tiles: Record<string, never> }
    tileLayer._tiles = {}

    await wrapper.setProps({ view: mapView(new Date('2026-08-19T12:00:05Z')) })
    await flushPromises()

    for (const group of leafletMocks.layerGroups) {
      expect(group.clearLayers).toHaveBeenCalledTimes(2)
    }
    wrapper.unmount()
  })

  it('shows tactical layers and preserves player follow in public rendering', async () => {
    const publicView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      players: [
        { id: 'public-1', name: 'Alex', online: true, position: { x: 10, z: 20 } },
        { id: 'public-2', name: 'Alex', position: { x: 30, z: 40 } },
      ],
      bloodMoon: { active: true, gameTime: { day: 7, hour: 22 } },
      bloodMoonState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      claims: [{ ownerName: 'Private owner', position: { x: 40, z: 50 }, size: 41 }],
      claimsState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animals: [{ name: 'Private wolf', position: { x: 20, z: 30 } }],
      hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostiles: [{ name: 'Private zombie', position: { x: 60, z: 70 } }],
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      global: { renderStubDefaultSlot: true },
      props: { publicIdentifier: 'shared-map', view: publicView },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="map-tools"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="map-data-disclosure"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="tactical-layer-controls"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="blood-moon-status"]').exists()).toBe(true)
    expect(wrapper.attributes('data-blood-moon-active')).toBe('true')
    expect(leafletMocks.overlays.some((entry) => entry.kind === 'claim')).toBe(true)
    expect(
      leafletMocks.overlays.some((entry) => entry.popup?.textContent?.includes('Private')),
    ).toBe(true)

    await wrapper.get('[data-testid="focus-player-public-2"]').trigger('click')
    expect(leafletMocks.setView).toHaveBeenLastCalledWith([30, 40], 4, { animate: true })
    const publicMarkers = leafletMocks.overlays.filter((entry) => entry.title === 'Follow Alex')
    expect(publicMarkers).toHaveLength(2)
    publicMarkers[1]?.events?.click?.()
    await wrapper.vm.$nextTick()
    expect(leafletMocks.panTo).toHaveBeenLastCalledWith([30, 40], { animate: true })
    expect(wrapper.find('[data-testid="stop-following-player"]').exists()).toBe(true)

    await wrapper.setProps({
      view: create(SevenDaysToDieMapViewSchema, {
        ...publicView,
        players: [publicView.players[0], { ...publicView.players[1], position: { x: 50, z: 60 } }],
      }),
    })
    await flushPromises()
    expect(leafletMocks.panTo).toHaveBeenLastCalledWith([50, 60], { animate: true })

    const overlayLayer = leafletMocks.layerGroups[1]
    const overlayClearCount = overlayLayer?.clearLayers.mock.calls.length ?? 0
    const overlayCount = leafletMocks.overlays.length
    await wrapper.setProps({ loadError: true })
    await flushPromises()
    expect(overlayLayer?.clearLayers).toHaveBeenCalledTimes(overlayClearCount + 1)
    expect(leafletMocks.overlays).toHaveLength(overlayCount)
    expect(wrapper.find('[data-testid="tactical-layer-controls"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="blood-moon-status"]').exists()).toBe(false)
    expect(wrapper.attributes('data-blood-moon-active')).toBe('false')
    expect(wrapper.find('[data-testid="stop-following-player"]').exists()).toBe(true)
    wrapper.unmount()

    const viewOnlyWrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: { view: mapView(new Date('2026-08-19T12:00:00Z')) },
    })
    await flushPromises()
    expect(viewOnlyWrapper.find('[data-testid="tactical-layer-controls"]').exists()).toBe(false)
    viewOnlyWrapper.unmount()
  })

  it('reports unavailable private layers without rendering stale entity arrays', async () => {
    const unavailableView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      animals: [{ name: 'Stale wolf', position: { x: 20, z: 30 } }],
      bloodMoonState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      claimsState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
      hostileState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
      hostiles: [{ name: 'Stale zombie', position: { x: 60, z: 70 } }],
      nativeMarkerState:
        SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, { props: { view: unavailableView } })
    await flushPromises()

    const stateCases = [
      { layer: 'Land claims', label: 'Upstream access denied' },
      { layer: 'Blood Moon', label: 'Unavailable' },
      { layer: 'Hostiles', label: 'Upstream access denied' },
      { layer: 'Animals', label: 'Unavailable' },
    ]
    expect(wrapper.findAll('[data-testid^="toggle-"]')).toHaveLength(0)
    const mapData = wrapper.get('[data-testid="map-data-disclosure"]')
    for (const test of stateCases) {
      expect(mapData.text()).toContain(test.layer)
      expect(mapData.text()).toContain(test.label)
    }
    expect(mapData.text()).toContain('review the Web Dashboard configuration')
    expect(leafletMocks.overlays.some((entry) => entry.popup?.textContent?.includes('Stale'))).toBe(
      false,
    )
    wrapper.unmount()
  })
})
