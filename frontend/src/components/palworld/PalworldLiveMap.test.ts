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
  setLatLng: ReturnType<typeof vi.fn>
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

    setLatLng(): this {
      return this
    }

    setStyle(): this {
      return this
    }

    setTooltipContent(): this {
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
    removeLayer: vi.fn(),
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
        setLatLng: vi.fn(),
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
        setLatLng: record.setLatLng,
        setTooltipContent: vi.fn(() => marker),
      }
      leafletMocks.markerRecords.push(record)
      return marker
    }),
    tileLayer: vi.fn(() => ({ addTo: vi.fn() })),
  }

  return { default: leaflet }
})

const mountedWrappers: VueWrapper[] = []

function actor(
  key: string,
  name: string,
  locationX = 100,
  locationY = 200,
  kind = PalworldMapActorKind.PLAYER,
): PalworldMapActor {
  return create(PalworldMapActorSchema, {
    key,
    kind,
    name,
    locationX,
    locationY,
    active: true,
  })
}

function mountMap(actors: PalworldMapActor[], partial = false, available = true): VueWrapper {
  const wrapper = shallowMount(PalworldLiveMap, {
    props: {
      view: create(PalworldMapViewSchema, {
        actors,
        available,
        partial,
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

  it('moves an existing marker when a refreshed snapshot changes its position', async () => {
    const wrapper = mountMap([actor('player-1', 'Alex')])
    await flushPromises()

    const alex = latestMarker('Alex')
    await wrapper.setProps({
      view: create(PalworldMapViewSchema, {
        actors: [actor('player-1', 'Alex', 350, 450)],
        available: true,
        serverOnline: true,
      }),
    })
    await flushPromises()

    expect(alex.setLatLng).toHaveBeenLastCalledWith({ latitude: 350, longitude: 450 })
    expect(leafletMocks.markerRecords.filter((marker) => marker.title === 'Alex')).toHaveLength(1)
  })

  it('shows only player controls for a players-only snapshot', async () => {
    const wrapper = mountMap(
      [actor('player-1', 'Alex'), actor('base-1', 'Skyforge', 300, 400, PalworldMapActorKind.BASE)],
      true,
    )
    await flushPromises()

    expect(wrapper.text()).toContain('Players')
    expect(wrapper.text()).not.toContain('World actors')
    expect(wrapper.text()).not.toContain('Bases')
    expect(wrapper.text()).not.toContain('More actors')
  })

  it('does not advertise actor layers before a snapshot is available', async () => {
    const wrapper = mountMap([], false, false)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Players')
    expect(wrapper.text()).not.toContain('Bases')
    expect(wrapper.text()).not.toContain('More actors')
  })

  it('retains full actor controls for a world snapshot', async () => {
    const wrapper = mountMap([
      actor('player-1', 'Alex'),
      actor('base-1', 'Skyforge', 300, 400, PalworldMapActorKind.BASE),
    ])
    await flushPromises()

    expect(wrapper.text()).toContain('World actors')
    expect(wrapper.text()).toContain('Bases')
    expect(wrapper.text()).toContain('More actors')
  })
})
