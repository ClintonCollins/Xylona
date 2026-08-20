import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieMapPlayerSchema,
  SevenDaysToDieMapVectorSchema,
  SevenDaysToDieMapViewSchema,
} from '@/proto/xylona_pb'
import SevenDaysToDieLiveMap from './SevenDaysToDieLiveMap.vue'

const leafletMocks = vi.hoisted(() => ({
  layers: [] as object[],
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
  const layerGroup = {
    addTo: vi.fn(() => layerGroup),
    clearLayers: vi.fn(),
  }
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
  const marker = {
    addTo: vi.fn(() => marker),
    bindPopup: vi.fn(() => marker),
    bindTooltip: vi.fn(() => marker),
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
      layerGroup: vi.fn(() => layerGroup),
      map: vi.fn(() => map),
      marker: vi.fn(() => marker),
      point: vi.fn(),
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
      props: { view: mapView(new Date('2026-08-19T12:00:00Z')) },
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
})
