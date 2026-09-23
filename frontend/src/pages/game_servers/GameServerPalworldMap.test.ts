import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  PalworldMapLayerSchema,
  PalworldMapViewSchema,
  type PalworldMapLayer,
  type UpdatePalworldMapConfigRequest,
} from '@/proto/xylona_pb'
import GameServerPalworldMap from './GameServerPalworldMap.vue'

const mocks = vi.hoisted(() => ({
  dialog: vi.fn(),
  getMap: vi.fn(),
  updateConfig: vi.fn(),
  installTiles: vi.fn(),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getPalworldMap: mocks.getMap,
    updatePalworldMapConfig: mocks.updateConfig,
    installPalworldMapTiles: mocks.installTiles,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return { ...actual, useQuasar: () => ({ dialog: mocks.dialog }) }
})

vi.mock('@/api/notifications', () => ({
  notifyError: mocks.notifyError,
  notifySuccess: mocks.notifySuccess,
}))

const passThrough = { template: '<div><slot /></div>' }
const stubs = {
  PalworldLiveMap: true,
  GameServerMapShareSettings: true,
  PageHeader: { template: '<div><slot name="actions" /></div>' },
  'q-dialog': { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' },
  'q-card': passThrough,
  'q-card-section': passThrough,
  'q-card-actions': passThrough,
  'q-expansion-item': passThrough,
  'q-icon': true,
  'q-separator': true,
  'q-btn': {
    props: ['label', 'disable', 'loading'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
  },
  'q-input': {
    props: ['modelValue', 'label'],
    emits: ['update:modelValue'],
    template:
      '<input :aria-label="label" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}

function layer(id: string, label: string, tileUrlTemplate: string): PalworldMapLayer {
  return create(PalworldMapLayerSchema, { id, label, tileUrlTemplate, maxZoom: 6, tileSize: 512 })
}

const customLayers = () => [
  layer('palpagos', 'Palpagos', 'https://tiles.example.com/palpagos/{z}/{x}/{y}.png'),
  layer('world-tree', 'World Tree', 'https://tiles.example.com/tree/{z}/{x}/{y}.png'),
]
const managedLayers = () => [
  layer('palpagos', 'Palpagos', '/palworld-map-tiles/palpagos/{z}/{x}/{y}.png'),
  layer('world-tree', 'World Tree', '/palworld-map-tiles/tree/{z}/{x}/{y}.png'),
]

async function mountPage(layers: PalworldMapLayer[]) {
  mocks.getMap.mockResolvedValue({
    map: create(PalworldMapViewSchema, { canManageShare: true, layers }),
  })
  const wrapper = mount(GameServerPalworldMap, { global: { stubs } })
  await flushPromises()
  await button(wrapper, 'Map imagery').trigger('click')
  return wrapper
}

function button(wrapper: ReturnType<typeof mount>, label: string) {
  const found = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  if (!found) throw new Error(`button ${label} not found`)
  return found
}

function savedLayers(): PalworldMapLayer[] {
  const request = mocks.updateConfig.mock.calls[0][0] as UpdatePalworldMapConfigRequest
  return request.layers
}

describe('GameServerPalworldMap imagery settings', () => {
  let wrapper: ReturnType<typeof mount> | undefined

  beforeEach(() => {
    mocks.getMap.mockReset()
    mocks.updateConfig
      .mockReset()
      .mockImplementation((request: UpdatePalworldMapConfigRequest) =>
        Promise.resolve({ layers: request.layers }),
      )
    mocks.dialog.mockReset().mockImplementation(() => ({
      onOk: (confirm: () => void) => confirm(),
    }))
    mocks.notifyError.mockReset()
    mocks.notifySuccess.mockReset()
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
  })

  it('keeps every other layer when the first custom layer is edited', async () => {
    const layers = customLayers()
    wrapper = await mountPage(layers)

    expect(button(wrapper, 'Save').attributes('disabled')).toBeDefined()
    await wrapper.get('input[aria-label="Map label"]').setValue('Palpagos (edited)')
    expect(button(wrapper, 'Save').attributes('disabled')).toBeUndefined()
    // The form edits a copy, so the live map keeps its layers until Save.
    expect(layers[0].label).toBe('Palpagos')

    await button(wrapper, 'Save').trigger('click')
    await flushPromises()

    expect(savedLayers().map((saved) => saved.label)).toEqual(['Palpagos (edited)', 'World Tree'])
    expect(savedLayers()[1].tileUrlTemplate).toBe(layers[1].tileUrlTemplate)
  })

  it('keeps managed tiles active until a custom source is opened and changed', async () => {
    wrapper = await mountPage(managedLayers())

    expect(wrapper.text()).toContain('Active source: Palpagos and World Tree')
    expect(wrapper.find('input[aria-label="Map label"]').exists()).toBe(false)
    expect(button(wrapper, 'Save').attributes('disabled')).toBeDefined()

    await button(wrapper, 'Use a custom tile source instead').trigger('click')
    expect(button(wrapper, 'Save').attributes('disabled')).toBeDefined()
    await wrapper
      .get('input[aria-label="Tile URL template"]')
      .setValue('https://tiles.example.com/{z}/{x}/{y}.png')
    expect(button(wrapper, 'Save').attributes('disabled')).toBeUndefined()

    await button(wrapper, 'Save').trigger('click')
    await flushPromises()

    expect(savedLayers()).toHaveLength(1)
    expect(savedLayers()[0].tileUrlTemplate).toBe('https://tiles.example.com/{z}/{x}/{y}.png')
  })

  it('asks before the coordinate grid removes both layers', async () => {
    wrapper = await mountPage(managedLayers())

    await button(wrapper, 'Use coordinate grid').trigger('click')
    await flushPromises()

    expect(mocks.dialog).toHaveBeenCalledOnce()
    expect(mocks.dialog.mock.calls[0][0].message).toContain('all 2 map imagery layers')
    expect(mocks.dialog.mock.calls[0][0].message).toContain('Palpagos and World Tree')
    expect(savedLayers()).toEqual([])
  })
})
