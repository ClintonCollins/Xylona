import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, SteamAppDetailsSchema } from '@/proto/shared_pb'
import SteamAppSearch from './SteamAppSearch.vue'

const mocks = vi.hoisted(() => ({
  getSteamAppDetails: vi.fn(),
  listGames: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getSteamAppDetails: mocks.getSteamAppDetails,
    listGames: mocks.listGames,
  }),
}))

const QBtnStub = defineComponent({
  props: {
    label: { type: String, default: '' },
    to: { type: String, default: '' },
  },
  emits: ['click'],
  template: '<button :data-to="to" @click="$emit(\'click\')">{{ label }}</button>',
})

const QInputStub = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  emits: ['update:modelValue'],
  template:
    '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})

const passThrough = { template: '<div><slot name="avatar" /><slot /><slot name="action" /></div>' }

function mountSearch() {
  return mount(SteamAppSearch, {
    global: {
      stubs: {
        'q-btn': QBtnStub,
        'q-input': QInputStub,
        'q-card': passThrough,
        'q-card-section': passThrough,
        'q-card-actions': passThrough,
        'q-banner': passThrough,
        'q-badge': true,
        'q-icon': true,
      },
    },
  })
}

async function lookUp(wrapper: ReturnType<typeof mountSearch>, appId: string) {
  await wrapper.get('input').setValue(appId)
  const lookUpButton = wrapper.findAll('button').find((button) => button.text() === 'Look Up')
  await lookUpButton?.trigger('click')
  await flushPromises()
}

describe('SteamAppSearch', () => {
  beforeEach(() => {
    mocks.getSteamAppDetails.mockReset()
    mocks.listGames.mockReset()
    mocks.getSteamAppDetails.mockResolvedValue({
      detailsAvailable: true,
      details: create(SteamAppDetailsSchema, {
        appId: '896660',
        name: 'Valheim Dedicated Server',
      }),
    })
  })

  it('points to the catalog game instead of offering a duplicate', async () => {
    mocks.listGames.mockResolvedValue({
      games: [
        create(GameSchema, {
          id: 'valheim',
          name: 'Valheim',
          steamAppid: '896660',
          xylonaOfficial: true,
        }),
        create(GameSchema, { id: 'minecraft', name: 'Minecraft', steamAppid: '' }),
      ],
    })

    const wrapper = mountSearch()
    await lookUp(wrapper, '896660')

    const match = wrapper.get('[data-test="catalog-match"]')
    expect(match.text()).toContain('Already in your catalog: Valheim (Official)')
    expect(wrapper.find('[data-to="/games/valheim/edit"]').exists()).toBe(true)
    expect(wrapper.find('[data-to="/games/valheim/copy"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Use This Server')
  })

  it('offers the server when no catalog game uses the AppID', async () => {
    mocks.listGames.mockResolvedValue({
      games: [create(GameSchema, { id: 'minecraft', name: 'Minecraft', steamAppid: '' })],
    })

    const wrapper = mountSearch()
    await lookUp(wrapper, '896660')

    expect(wrapper.find('[data-test="catalog-match"]').exists()).toBe(false)
    const useButton = wrapper.findAll('button').find((b) => b.text() === 'Use This Server')
    await useButton?.trigger('click')
    expect(wrapper.emitted('select')).toEqual([
      [{ appId: '896660', name: 'Valheim Dedicated Server' }],
    ])
  })
})
