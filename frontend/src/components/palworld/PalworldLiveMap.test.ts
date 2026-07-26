import { create } from '@bufbuild/protobuf'
import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { nextTick } from 'vue'

import {
  PalworldMapActorKind,
  PalworldMapActorSchema,
  PalworldMapHealthSchema,
  PalworldMapLayerSchema,
  PalworldMapViewSchema,
  type PalworldMapActor,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import PalworldLiveMap from './PalworldLiveMap.vue'

interface MarkerRecord {
  bindTooltip: Mock<(...args: unknown[]) => void>
  click: (() => void) | null
  element: HTMLElement
  openTooltip: Mock<() => void>
  options: { zIndexOffset?: number }
  setLatLng: ReturnType<typeof vi.fn>
  title: string
}

const leafletMocks = vi.hoisted(() => ({
  animationFrame: vi.fn((callback: FrameRequestCallback) => {
    queueMicrotask(() => callback(0))
    return 1
  }),
  circleMarker: vi.fn(),
  clearLayers: vi.fn(),
  shapeMarker: vi.fn(),
  fitBounds: vi.fn(),
  getZoom: vi.fn(() => 1),
  invalidateSize: vi.fn(),
  mapEventHandlers: new Map<string, () => void>(),
  mapOptions: [] as Record<string, unknown>[],
  markerRecords: [] as MarkerRecord[],
  tileLayerOptions: [] as Record<string, unknown>[],
  // Screen pixels per world unit, so fixture coordinates spaced further apart
  // than a pill are genuinely separate to the label planner.
  project: vi.fn((location: { latitude: number; longitude: number }) => ({
    x: location.latitude,
    y: location.longitude,
  })),
  setView: vi.fn(),
}))

vi.mock('leaflet', () => {
  class CircleMarker {
    // Mirrors Leaflet's own class factory so the canvas shape marker, which
    // subclasses CircleMarker to draw diamonds and squares, can be built here.
    static extend(properties: Record<string, unknown>): typeof CircleMarker {
      const Subclass = class extends CircleMarker {
        constructor(...args: unknown[]) {
          super()
          leafletMocks.shapeMarker(...args)
        }
      }
      Object.assign(Subclass.prototype, properties)
      return Subclass
    }

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
    invalidateSize: leafletMocks.invalidateSize,
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
    circleMarker: vi.fn(() => {
      leafletMocks.circleMarker()
      return new CircleMarker()
    }),
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
    map: vi.fn((_element: unknown, options: Record<string, unknown>) => {
      leafletMocks.mapOptions.push(options)
      return mapInstance
    }),
    marker: vi.fn(
      (
        _location: unknown,
        options: { icon: { html: string }; title: string; zIndexOffset?: number },
      ) => {
        const element = document.createElement('div')
        element.innerHTML = options.icon.html
        const record: MarkerRecord = {
          bindTooltip: vi.fn<(...args: unknown[]) => void>(),
          click: null,
          element,
          openTooltip: vi.fn<() => void>(),
          options,
          setLatLng: vi.fn(),
          title: options.title,
        }
        const marker = {
          addTo: vi.fn(() => marker),
          bindTooltip: vi.fn((...args: unknown[]) => {
            record.bindTooltip(...args)
            return marker
          }),
          getElement: vi.fn(() => element),
          on: vi.fn((event: string, handler: () => void) => {
            if (event === 'click') {
              record.click = handler
            }
            return marker
          }),
          openTooltip: vi.fn(() => {
            record.openTooltip()
            return marker
          }),
          setLatLng: record.setLatLng,
          setTooltipContent: vi.fn(() => marker),
          setZIndexOffset: vi.fn(() => marker),
        }
        leafletMocks.markerRecords.push(record)
        return marker
      },
    ),
    tileLayer: vi.fn((_url: string, options: Record<string, unknown>) => {
      leafletMocks.tileLayerOptions.push(options)
      return { addTo: vi.fn() }
    }),
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

function mountMap(
  actors: PalworldMapActor[],
  partial = false,
  available = true,
  partialReason = '',
): VueWrapper {
  const wrapper = shallowMount(PalworldLiveMap, {
    props: {
      view: create(PalworldMapViewSchema, {
        actors,
        available,
        partial,
        partialReason,
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
    leafletMocks.shapeMarker.mockClear()
    leafletMocks.mapOptions.length = 0
    leafletMocks.tileLayerOptions.length = 0
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

  it.each([
    {
      name: 'explains a players-only snapshot with the reported reason',
      partial: true,
      partialReason: 'Add -enable-gamedata-api to this server start arguments and restart it.',
      expected: '-enable-gamedata-api',
    },
    {
      name: 'explains a players-only snapshot even when no reason was reported',
      partial: true,
      partialReason: '',
      expected: 'player positions only',
    },
    {
      name: 'stays hidden on a complete world snapshot',
      partial: false,
      partialReason: '',
      expected: '',
    },
  ])('$name', async ({ partial, partialReason, expected }) => {
    const wrapper = mountMap([actor('player-1', 'Alex')], partial, true, partialReason)
    await flushPromises()

    const notice = wrapper.find('.palworld-live-map__partial')
    if (expected === '') {
      expect(notice.exists()).toBe(false)
      return
    }
    expect(notice.text()).toContain(expected)
  })

  it('frames visible actors with viewport-safe padding on a narrow map', async () => {
    const clientWidth = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(320)
    try {
      mountMap([actor('player-1', 'Alex')])
      await flushPromises()

      // 6 is the fallback grid's native ceiling: the opening fit never lands on
      // the upscaled levels, which are reserved for explicit zoom and focus.
      const actorFit = leafletMocks.fitBounds.mock.calls.find(
        ([, options]) => (options as { maxZoom?: number } | undefined)?.maxZoom === 6,
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
    mountMap([actor('player-1', 'Alex'), actor('player-2', 'Sam', 400, 500)])
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
    expect(wrapper.text()).toContain('Base Pals')
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

  it('skips the render when a poll repeats the snapshot it already drew', async () => {
    const collectedAt = new Date('2026-07-25T03:00:00.000Z')
    const snapshot = (): PalworldMapView =>
      create(PalworldMapViewSchema, {
        actors: [actor('player-1', 'Alex')],
        available: true,
        serverOnline: true,
        collectedAt: timestampFromDate(collectedAt),
      })
    const wrapper = shallowMount(PalworldLiveMap, { props: { view: snapshot() } })
    mountedWrappers.push(wrapper)
    await flushPromises()

    const alex = latestMarker('Alex')
    const movesAfterMount = alex.setLatLng.mock.calls.length

    await wrapper.setProps({ view: snapshot() })
    await flushPromises()

    expect(alex.setLatLng.mock.calls.length).toBe(movesAfterMount)
    expect(leafletMocks.markerRecords.filter((marker) => marker.title === 'Alex')).toHaveLength(1)
  })

  it('does not force a Leaflet resize when a new snapshot arrives', async () => {
    const wrapper = mountMap([actor('player-1', 'Alex')])
    await flushPromises()
    const resizesAfterMount = leafletMocks.invalidateSize.mock.calls.length

    await wrapper.setProps({
      view: create(PalworldMapViewSchema, {
        actors: [actor('player-1', 'Alex', 350, 450)],
        available: true,
        serverOnline: true,
      }),
    })
    await flushPromises()

    expect(leafletMocks.invalidateSize.mock.calls.length).toBe(resizesAfterMount)
  })

  it('mutates a marker in place when only its active state changes', async () => {
    const wrapper = mountMap([actor('player-1', 'Alex')])
    await flushPromises()

    const alex = latestMarker('Alex')
    const pill = (): DOMTokenList | undefined =>
      alex.element.querySelector('.palworld-map-marker')?.classList
    expect(pill()?.contains('palworld-map-marker--active')).toBe(true)

    await wrapper.setProps({
      view: create(PalworldMapViewSchema, {
        actors: [
          actor('player-1', 'Alex', 100, 200, PalworldMapActorKind.PLAYER, { active: false }),
        ],
        available: true,
        serverOnline: true,
      }),
    })
    await flushPromises()

    expect(leafletMocks.markerRecords.filter((marker) => marker.title === 'Alex')).toHaveLength(1)
    expect(pill()?.contains('palworld-map-marker--active')).toBe(false)
  })

  it('folds base workers into a count on their base instead of separate markers', async () => {
    const guild = { guildKey: 'guild-1', guildName: 'Skyforge' }
    mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE, guild),
      actor('worker-1', 'Anubis', 110, 210, PalworldMapActorKind.BASE_WORKER, guild),
      actor('worker-2', 'Chikipi', 120, 220, PalworldMapActorKind.BASE_WORKER, guild),
    ])
    await flushPromises()

    expect(leafletMocks.circleMarker).not.toHaveBeenCalled()
    expect(leafletMocks.markerRecords.some((marker) => marker.title.includes('base worker'))).toBe(
      false,
    )
    expect(latestMarker('North Camp').element.textContent).toContain('2')
  })

  it('lists the pals working at the selected base', async () => {
    const guild = { guildKey: 'guild-1', guildName: 'Skyforge' }
    const wrapper = mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE, guild),
      actor('worker-1', 'Anubis', 110, 210, PalworldMapActorKind.BASE_WORKER, guild),
    ])
    await flushPromises()

    latestMarker('North Camp').click?.()
    await flushPromises()

    expect(wrapper.text()).toContain('Base Pals')
    expect(wrapper.text()).toContain('Anubis')
  })

  it('hides a colliding base label and restores it on selection without rebuilding', async () => {
    // 60px apart: the names collide but the icon boxes still clear each other,
    // so this stays on the compact rung rather than merging.
    mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE),
      actor('base-2', 'South Camp', 160, 200, PalworldMapActorKind.BASE),
    ])
    await flushPromises()

    const south = latestMarker('South Camp')
    const pill = (): DOMTokenList | undefined =>
      south.element.querySelector('.palworld-map-marker')?.classList
    expect(pill()?.contains('palworld-map-marker--compact')).toBe(true)

    south.click?.()
    await flushPromises()

    expect(pill()?.contains('palworld-map-marker--compact')).toBe(false)
    expect(
      leafletMocks.markerRecords.filter((marker) => marker.title === 'South Camp'),
    ).toHaveLength(1)
  })

  it('extends the map past the tile source and steps a whole level at a time', async () => {
    mountMap([actor('player-1', 'Alex')])
    await flushPromises()

    // The coordinate-grid fallback declares maxZoom 6, so the map reaches 9.
    expect(leafletMocks.mapOptions.at(-1)).toMatchObject({
      maxZoom: 9,
      minZoom: 0,
      zoomDelta: 1,
      zoomSnap: 0.25,
    })
  })

  it('caps tile requests at the last real level while the map keeps zooming', async () => {
    const wrapper = shallowMount(PalworldLiveMap, {
      props: {
        view: create(PalworldMapViewSchema, {
          actors: [actor('player-1', 'Alex')],
          available: true,
          serverOnline: true,
          layers: [
            create(PalworldMapLayerSchema, {
              id: 'default',
              label: 'Palpagos',
              tileUrlTemplate: '/api/palworld-map/default/{z}/{x}/{y}.webp',
              attribution: 'Palworld © Pocketpair',
              minZoom: 0,
              maxZoom: 4,
              tileSize: 512,
              transformA: 0.00035,
              transformB: 256,
              transformC: -0.00035,
              transformD: 256,
              minX: -600_000,
              minY: -600_000,
              maxX: 600_000,
              maxY: 600_000,
            }),
          ],
        }),
      },
    })
    mountedWrappers.push(wrapper)
    await flushPromises()

    expect(leafletMocks.mapOptions.at(-1)).toMatchObject({ maxZoom: 7 })
    expect(leafletMocks.tileLayerOptions.at(-1)).toMatchObject({
      maxNativeZoom: 4,
      maxZoom: 7,
    })
  })

  it('flies to the focus zoom when an actor is picked from the base roster', async () => {
    const guild = { guildKey: 'guild-1', guildName: 'Skyforge' }
    const wrapper = mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE, guild),
      actor('worker-1', 'Anubis', 110, 210, PalworldMapActorKind.BASE_WORKER, guild),
    ])
    await flushPromises()

    latestMarker('North Camp').click?.()
    await flushPromises()
    await wrapper.find('.palworld-live-map__base-pals button').trigger('click')
    await flushPromises()

    expect(leafletMocks.setView).toHaveBeenLastCalledWith(
      { latitude: 110, longitude: 210 },
      8,
      expect.objectContaining({ animate: true }),
    )
  })

  it('merges pills whose icons would stack and drills to the deepest zoom', async () => {
    mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE),
      actor('base-2', 'South Camp', 100, 200, PalworldMapActorKind.BASE),
    ])
    await flushPromises()

    const merge = latestMarker('2 bases')
    expect(merge.element.textContent).toContain('2')
    expect(merge.options.zIndexOffset).toBe(1000)
    expect(leafletMocks.markerRecords.some((record) => record.title === 'North Camp')).toBe(false)

    merge.click?.()
    expect(leafletMocks.fitBounds).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.objectContaining({ animate: true, maxZoom: 9 }),
    )
  })

  it('swaps dots for icon markers once the map is past the tile source', async () => {
    leafletMocks.getZoom.mockReturnValue(7)
    mountMap([actor('wild-1', 'Lamball', 100, 200, PalworldMapActorKind.WILD_PAL)])
    await flushPromises()

    expect(leafletMocks.shapeMarker).not.toHaveBeenCalled()
    const dot = latestMarker('Lamball')
    expect(dot.element.querySelector('.palworld-map-dot--icon')).not.toBeNull()
    // Crossing into icon markers must not cost the coordinate readout on hover.
    expect(dot.bindTooltip).toHaveBeenCalled()
  })

  it('opens the focused actor tooltip on the marker that survives the zoom', async () => {
    const guild = { guildKey: 'guild-1', guildName: 'Skyforge' }
    const wrapper = mountMap([
      actor('base-1', 'North Camp', 100, 200, PalworldMapActorKind.BASE, guild),
      actor('worker-1', 'Anubis', 110, 210, PalworldMapActorKind.BASE_WORKER, guild),
    ])
    await flushPromises()

    latestMarker('North Camp').click?.()
    await flushPromises()
    await wrapper.find('.palworld-live-map__base-pals button').trigger('click')
    await flushPromises()

    // The focus zoom of 8 is past the fallback grid's native 6, so the canvas dot
    // is torn down and replaced. The tooltip must land on the replacement.
    leafletMocks.getZoom.mockReturnValue(8)
    leafletMocks.mapEventHandlers.get('zoomend')?.()
    await flushPromises()

    const icon = latestMarker('Anubis')
    expect(icon.element.querySelector('.palworld-map-dot--icon')).not.toBeNull()
    expect(icon.openTooltip).toHaveBeenCalled()
  })

  it('keeps dots on the canvas renderer at and below the tile source', async () => {
    leafletMocks.getZoom.mockReturnValue(4)
    mountMap([actor('wild-1', 'Lamball', 100, 200, PalworldMapActorKind.WILD_PAL)])
    await flushPromises()

    expect(leafletMocks.shapeMarker).toHaveBeenCalled()
    expect(leafletMocks.markerRecords.some((record) => record.title === 'Lamball')).toBe(false)
  })

  it('stacks players above bases and keeps clusters beneath both', async () => {
    mountMap([
      actor('player-1', 'Alex'),
      actor('base-1', 'North Camp', 300, 400, PalworldMapActorKind.BASE),
      actor('wild-1', 'Lamball', 505, 605, PalworldMapActorKind.WILD_PAL),
      actor('wild-2', 'Cattiva', 510, 610, PalworldMapActorKind.WILD_PAL),
    ])
    await flushPromises()

    expect(latestMarker('Alex').options.zIndexOffset).toBe(2000)
    expect(latestMarker('North Camp').options.zIndexOffset).toBe(1000)
    expect(latestMarker('2 wild pals').options.zIndexOffset).toBe(-1000)
  })
})
