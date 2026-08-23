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
  overlays: [] as { group?: object; kind: string; popup?: HTMLElement; title?: string }[],
  redraw: vi.fn(),
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
    invalidateSize: vi.fn(),
    project: vi.fn(() => ({
      unscaleBy: vi.fn(() => ({
        floor: vi.fn(() => ({ x: 1, y: -2 })),
      })),
    })),
    remove: vi.fn(),
    setView: vi.fn(),
  }
  function overlay(kind: string, options?: { title?: string }) {
    const record: { group?: object; kind: string; popup?: HTMLElement; title?: string } = {
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
    }
    return result
  }

  return {
    default: {
      CRS: { Simple: {} },
      GridLayer,
      Transformation: class {},
      bounds: vi.fn(() => bounds),
      divIcon: vi.fn(),
      extend: Object.assign,
      latLngBounds: vi.fn(() => bounds),
      circleMarker: vi.fn(() => overlay('animal-or-hostile')),
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
    leafletMocks.redraw.mockClear()
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

  it('keeps tactical controls independent on one shared Leaflet layer and escapes labels', async () => {
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
    const wrapper = shallowMount(SevenDaysToDieLiveMap, { props: { view: tacticalView } })
    await flushPromises()

    expect(wrapper.get('[data-testid="tactical-layer-controls"]').attributes('role')).toBe('group')
    expect(wrapper.findAll('[data-testid^="toggle-"]')).toHaveLength(5)
    expect(wrapper.get('[data-testid="blood-moon-overlay"]').text()).toContain('Blood Moon active')
    expect(leafletMocks.layerGroups).toHaveLength(2)
    expect(leafletMocks.overlays.some((entry) => entry.kind === 'claim')).toBe(true)
    expect(
      leafletMocks.overlays.filter((entry) => entry.kind === 'animal-or-hostile'),
    ).toHaveLength(2)
    const overlayLayer = leafletMocks.layerGroups[1]
    expect(leafletMocks.overlays.filter((entry) => entry.group === overlayLayer)).toHaveLength(4)
    const unsafeMarker = leafletMocks.overlays.find(
      (entry) => entry.title === '<img src=x onerror=alert(1)>',
    )
    expect(unsafeMarker?.popup?.textContent).toContain('<img src=x onerror=alert(1)>')
    expect(unsafeMarker?.popup?.querySelector('img')).toBeNull()

    const layerToggleCases = [
      'toggle-native-markers',
      'toggle-land-claims',
      'toggle-hostiles',
      'toggle-animals',
    ]
    for (const controlTestId of layerToggleCases) {
      const overlayCountBeforeToggle = leafletMocks.overlays.length
      const control = wrapper.getComponent(`[data-testid="${controlTestId}"]`)
      control.vm.$emit('update:modelValue', false)
      await flushPromises()
      const rebuiltOverlays = leafletMocks.overlays.slice(overlayCountBeforeToggle)
      expect(rebuiltOverlays).toHaveLength(3)
      expect(rebuiltOverlays.every((entry) => entry.group === overlayLayer)).toBe(true)
      expect(
        rebuiltOverlays.some((entry) => {
          switch (controlTestId) {
            case 'toggle-native-markers':
              return entry.title === '<img src=x onerror=alert(1)>'
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

    const overlayCount = leafletMocks.overlays.length
    const bloodMoonControl = wrapper.getComponent('[data-testid="toggle-blood-moon"]')
    bloodMoonControl.vm.$emit('update:modelValue', false)
    await flushPromises()
    expect(wrapper.find('[data-testid="blood-moon-overlay"]').exists()).toBe(false)
    expect(leafletMocks.overlays).toHaveLength(overlayCount)
    bloodMoonControl.vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(wrapper.find('[data-testid="blood-moon-overlay"]').exists()).toBe(true)
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

  it('omits tactical controls and layers from public rendering', async () => {
    const publicView = create(SevenDaysToDieMapViewSchema, {
      ...mapView(new Date('2026-08-19T12:00:00Z')),
      animalState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      animals: [{ name: 'Private wolf', position: { x: 20, z: 30 } }],
      hostileState: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      hostiles: [{ name: 'Private zombie', position: { x: 60, z: 70 } }],
    })
    const wrapper = shallowMount(SevenDaysToDieLiveMap, {
      props: { publicMode: true, view: publicView },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="tactical-layer-controls"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="blood-moon-overlay"]').exists()).toBe(false)
    expect(
      leafletMocks.overlays.some((entry) => entry.popup?.textContent?.includes('Private')),
    ).toBe(false)
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
      { control: 'toggle-native-markers', label: 'Unsupported' },
      { control: 'toggle-land-claims', label: 'Upstream access denied' },
      { control: 'toggle-blood-moon', label: 'Unavailable' },
      { control: 'toggle-hostiles', label: 'Upstream access denied' },
      { control: 'toggle-animals', label: 'Unavailable' },
    ]
    for (const test of stateCases) {
      const control = wrapper.get(`[data-testid="${test.control}"]`)
      expect(control.attributes('label')).toContain(test.label)
      expect(control.attributes('disable')).toBe('true')
    }
    expect(leafletMocks.overlays.some((entry) => entry.popup?.textContent?.includes('Stale'))).toBe(
      false,
    )
    wrapper.unmount()
  })
})
