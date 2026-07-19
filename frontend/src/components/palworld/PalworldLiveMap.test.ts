import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  PalworldMapActorKind,
  PalworldMapActorSchema,
  PalworldMapViewSchema,
  type PalworldMapActor,
} from '@/proto/xylona_pb'
import PalworldLiveMap from './PalworldLiveMap.vue'

interface MarkerRecord {
  click: (() => void) | null
  element: HTMLElement
  title: string
}

const leafletMocks = vi.hoisted(() => ({
  clearLayers: vi.fn(),
  fitBounds: vi.fn(),
  markerRecords: [] as MarkerRecord[],
}))

vi.mock('leaflet', () => {
  class CircleMarker {
    addTo(): this {
      return this
    }

    bindTooltip(): this {
      return this
    }

    getElement(): HTMLElement | null {
      return null
    }

    on(): this {
      return this
    }

    openTooltip(): this {
      return this
    }

    setRadius(): this {
      return this
    }

    setStyle(): this {
      return this
    }
  }

  const mapInstance = {
    fitBounds: leafletMocks.fitBounds,
    getZoom: vi.fn(() => 1),
    invalidateSize: vi.fn(),
    remove: vi.fn(),
    setView: vi.fn(),
    setZoom: vi.fn(),
  }

  const layerGroupInstance = {
    addTo: vi.fn(() => layerGroupInstance),
    clearLayers: leafletMocks.clearLayers,
  }

  const leaflet = {
    CRS: { Simple: {} },
    CircleMarker,
    Transformation: class {
      constructor(
        public readonly a: number,
        public readonly b: number,
        public readonly c: number,
        public readonly d: number,
      ) {}
    },
    canvas: vi.fn(() => ({})),
    circleMarker: vi.fn(() => new CircleMarker()),
    divIcon: vi.fn((options: { html: string }) => options),
    latLng: vi.fn((latitude: number, longitude: number) => ({ latitude, longitude })),
    latLngBounds: vi.fn(() => {
      const bounds = {
        contains: vi.fn(() => true),
        pad: vi.fn(() => bounds),
      }
      return bounds
    }),
    layerGroup: vi.fn(() => layerGroupInstance),
    map: vi.fn(() => mapInstance),
    marker: vi.fn((_location: unknown, options: { icon: { html: string }; title: string }) => {
      const element = document.createElement('div')
      element.innerHTML = options.icon.html
      const record: MarkerRecord = {
        click: null,
        element,
        title: options.title,
      }
      const marker = {
        addTo: vi.fn(() => marker),
        getElement: vi.fn(() => element),
        on: vi.fn((event: string, handler: () => void) => {
          if (event === 'click') {
            record.click = handler
          }
          return marker
        }),
        openTooltip: vi.fn(() => marker),
      }
      leafletMocks.markerRecords.push(record)
      return marker
    }),
    tileLayer: vi.fn(() => ({ addTo: vi.fn() })),
  }

  return { default: leaflet }
})

const mountedWrappers: VueWrapper[] = []

function actor(key: string, name: string): PalworldMapActor {
  return create(PalworldMapActorSchema, {
    key,
    kind: PalworldMapActorKind.PLAYER,
    name,
    locationX: 100,
    locationY: 200,
    active: true,
  })
}

function mountMap(actors: PalworldMapActor[]): VueWrapper {
  const wrapper = shallowMount(PalworldLiveMap, {
    props: {
      view: create(PalworldMapViewSchema, {
        actors,
        available: true,
        serverOnline: true,
      }),
    },
  })
  mountedWrappers.push(wrapper)
  return wrapper
}

function latestMarker(name: string): MarkerRecord {
  const marker = leafletMocks.markerRecords.findLast((candidate) => candidate.title === name)
  if (marker === undefined) {
    throw new Error(`expected marker for ${name}`)
  }
  return marker
}

describe('PalworldLiveMap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    leafletMocks.markerRecords.length = 0
    vi.stubGlobal(
      'ResizeObserver',
      class {
        disconnect(): void {}
        observe(): void {}
      },
    )
  })

  afterEach(() => {
    for (const wrapper of mountedWrappers.splice(0)) {
      wrapper.unmount()
    }
    vi.unstubAllGlobals()
  })

  it('frames visible actors after establishing the initial world bounds', async () => {
    mountMap([actor('player-1', 'Alex')])
    await flushPromises()

    const actorFit = leafletMocks.fitBounds.mock.calls.find(
      ([, options]) => (options as { maxZoom?: number } | undefined)?.maxZoom === 4,
    )
    expect(actorFit).toBeDefined()
  })

  it('updates only the previous and current marker when selection changes', async () => {
    mountMap([actor('player-1', 'Alex'), actor('player-2', 'Sam')])
    await flushPromises()

    const alex = latestMarker('Alex')
    const sam = latestMarker('Sam')
    const clearsBeforeSelection = leafletMocks.clearLayers.mock.calls.length

    alex.click?.()
    await nextTick()
    expect(leafletMocks.clearLayers).toHaveBeenCalledTimes(clearsBeforeSelection)
    expect(
      alex.element
        .querySelector('.palworld-map-marker')
        ?.classList.contains('palworld-map-marker--selected'),
    ).toBe(true)

    sam.click?.()
    await nextTick()
    expect(leafletMocks.clearLayers).toHaveBeenCalledTimes(clearsBeforeSelection)
    expect(
      alex.element
        .querySelector('.palworld-map-marker')
        ?.classList.contains('palworld-map-marker--selected'),
    ).toBe(false)
    expect(
      sam.element
        .querySelector('.palworld-map-marker')
        ?.classList.contains('palworld-map-marker--selected'),
    ).toBe(true)
  })
})
