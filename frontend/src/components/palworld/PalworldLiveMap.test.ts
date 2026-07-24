import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import {
  PalworldMapActorKind,
  PalworldMapActorSchema,
  PalworldMapHealthSchema,
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
  animationFrame: vi.fn((callback: FrameRequestCallback) => {
    queueMicrotask(() => callback(0))
    return 1
  }),
  clearLayers: vi.fn(),
  fitBounds: vi.fn(),
  getZoom: vi.fn(() => 1),
  mapEventHandlers: new Map<string, () => void>(),
  markerRecords: [] as MarkerRecord[],
  project: vi.fn((location: { latitude: number; longitude: number }) => ({
    x: location.latitude / 1_000,
    y: location.longitude / 1_000,
  })),
  setView: vi.fn(),
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
    getBounds: vi.fn(() => {
      const bounds = {
        getEast: vi.fn(() => 900_000),
        getNorth: vi.fn(() => 750_000),
        getSouth: vi.fn(() => -1_200_000),
        getWest: vi.fn(() => -900_000),
        pad: vi.fn(() => bounds),
      }
      return bounds
    }),
    getZoom: leafletMocks.getZoom,
    invalidateSize: vi.fn(),
    on: vi.fn((events: string, handler: () => void) => {
      for (const event of events.split(' ')) {
        leafletMocks.mapEventHandlers.set(event, handler)
      }
      return mapInstance
    }),
    project: leafletMocks.project,
    remove: vi.fn(),
    setView: leafletMocks.setView,
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
  details: Partial<PalworldMapActor> = {},
): PalworldMapActor {
  return Object.assign(
    create(PalworldMapActorSchema, {
      key,
      kind,
      name,
      locationX,
      locationY,
      active: true,
    }),
    details,
  )
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
    leafletMocks.mapEventHandlers.clear()
    leafletMocks.getZoom.mockReturnValue(1)
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    vi.stubGlobal('requestAnimationFrame', leafletMocks.animationFrame)
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

  it('frames visible actors with viewport-safe padding on a narrow map', async () => {
    const clientWidth = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(320)
    try {
      mountMap([actor('player-1', 'Alex')])
      await flushPromises()

      const actorFit = leafletMocks.fitBounds.mock.calls.find(
        ([, options]) => (options as { maxZoom?: number } | undefined)?.maxZoom === 4,
      )
      expect(actorFit?.[1]).toMatchObject({
        paddingBottomRight: [48, 80],
        paddingTopLeft: [48, 80],
      })
    } finally {
      clientWidth.mockRestore()
    }
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

  it('clusters dense actors at low zoom and fits a cluster on click', async () => {
    const wrapper = mountMap([
      actor('wild-1', 'Lamball', 100, 100, PalworldMapActorKind.WILD_PAL),
      actor('wild-2', 'Cattiva', 120, 120, PalworldMapActorKind.WILD_PAL),
    ])
    await flushPromises()

    const cluster = latestMarker('2 wild pals')
    expect(cluster.element.textContent).toContain('2')

    cluster.click?.()
    expect(leafletMocks.fitBounds).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ animate: true, maxZoom: 4 }),
    )

    const animationFrameCalls = leafletMocks.animationFrame.mock.calls.length
    leafletMocks.getZoom.mockReturnValue(4)
    leafletMocks.mapEventHandlers.get('moveend')?.()
    leafletMocks.mapEventHandlers.get('zoomend')?.()
    await flushPromises()

    expect(leafletMocks.animationFrame.mock.calls.length).toBe(animationFrameCalls + 1)
    expect(wrapper.text()).not.toContain('actors grouped at this zoom')
  })

  it('focuses a population summary and qualifies truncated counts', async () => {
    const wrapper = shallowMount(PalworldLiveMap, {
      props: {
        view: create(PalworldMapViewSchema, {
          actors: [
            actor('player-1', 'Alex'),
            actor('base-1', 'Skyforge', 300, 400, PalworldMapActorKind.BASE),
          ],
          available: true,
          serverOnline: true,
          truncated: true,
        }),
      },
    })
    mountedWrappers.push(wrapper)
    await flushPromises()

    expect(wrapper.text()).toContain('At least 2')
    const bases = wrapper
      .findAll('button')
      .find((button) => button.attributes('aria-label') === 'Focus bases')
    expect(bases).toBeDefined()
    await bases?.trigger('click')
    await flushPromises()

    expect(bases?.attributes('aria-pressed')).toBe('true')
    expect(leafletMocks.fitBounds).toHaveBeenCalled()
  })

  it('shows rich actor and guild-base intelligence without claiming assignments', async () => {
    const guildDetails = { guildKey: 'guild-1', guildName: 'Skyforge' }
    const wrapper = mountMap([
      actor('base-1', 'North Camp', 100, 100, PalworldMapActorKind.BASE, {
        ...guildDetails,
        hp: 420,
        maxHp: 500,
        level: 32,
        rotationZ: 91,
        className: 'Pal_Anubis',
        action: 'Crafting',
        aiAction: 'Work',
      }),
      actor('base-2', 'South Camp', 900, 900, PalworldMapActorKind.BASE, guildDetails),
      actor('worker-1', 'Anubis', 120, 120, PalworldMapActorKind.BASE_WORKER, {
        ...guildDetails,
        hp: 80,
        maxHp: 100,
        active: true,
      }),
    ])
    await flushPromises()

    latestMarker('North Camp').click?.()
    await flushPromises()

    expect(wrapper.text()).toContain('Guilds & bases')
    expect(wrapper.text()).toContain('Nearby worker estimate')
    expect(wrapper.text()).toContain('the official API does not report per-base assignments')
    expect(wrapper.text()).toContain('420 / 500')
    expect(wrapper.text()).toContain('FacingE 91°')
    expect(wrapper.text()).toContain('Class · Pal_Anubis')
    expect(wrapper.text()).toContain('Action · Crafting')
    expect(wrapper.text()).toContain('AI action · Work')
    expect(wrapper.text()).toContain('Active workers1')
    expect(wrapper.text()).toContain('Injured workers1')

    const focusGuild = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Focus guild'))
    await focusGuild?.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Clear focus')
  })

  it('labels guild-name fallback grouping as an estimate', async () => {
    const wrapper = mountMap([
      actor('base-1', 'North Camp', 100, 100, PalworldMapActorKind.BASE, {
        guildName: 'Skyforge',
      }),
      actor('worker-1', 'Anubis', 120, 120, PalworldMapActorKind.BASE_WORKER, {
        guildName: 'Skyforge',
      }),
    ])
    await flushPromises()

    latestMarker('North Camp').click?.()
    await flushPromises()

    expect(wrapper.text()).toContain('name-matched estimate')
    expect(wrapper.text()).toContain(
      'Guild identity is unavailable in this snapshot, so related actors are matched by guild name.',
    )
  })

  it('shows supplied world health and omits the strip when health is unavailable', async () => {
    const wrapper = shallowMount(PalworldLiveMap, {
      props: {
        view: create(PalworldMapViewSchema, {
          actors: [],
          available: true,
          serverOnline: true,
          collectedAt: timestampFromDate(new Date(Date.now() - 30 * 60 * 1_000)),
          health: create(PalworldMapHealthSchema, {
            serverFps: 59.6,
            serverFrameTimeMs: 16.8,
            currentPlayers: 3,
            maxPlayers: 32,
            baseCampCount: 7,
            days: 112,
            uptimeSeconds: 93_900n,
          }),
        }),
      },
    })
    mountedWrappers.push(wrapper)
    await flushPromises()

    expect(wrapper.find('.palworld-live-map__health').exists()).toBe(true)
    expect(wrapper.text()).toContain('3 / 32')
    expect(wrapper.text()).toContain('59.6 FPS · 16.8 ms')
    expect(wrapper.text()).toContain('1d 2h')
    expect(wrapper.text()).toContain('Updated 30m ago')

    await wrapper.setProps({
      view: create(PalworldMapViewSchema, {
        actors: [],
        available: true,
        serverOnline: true,
      }),
    })
    await flushPromises()
    expect(wrapper.find('.palworld-live-map__health').exists()).toBe(false)
  })
})
