import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GameConfigSchema from './GameConfigSchema.vue'

const mocks = vi.hoisted(() => ({
  getGameConfigSchemas: vi.fn(),
  updateGameConfigSchemas: vi.fn(),
  back: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({
      params: {
        id: 'minecraft',
        fileIndex: '0',
      },
    }),
    useRouter: () => ({
      back: mocks.back,
    }),
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getGameConfigSchemas: mocks.getGameConfigSchemas,
      updateGameConfigSchemas: mocks.updateGameConfigSchemas,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

const ConfigSchemaEditorStub = defineComponent({
  name: 'ConfigSchemaEditor',
  emits: ['save', 'back'],
  template:
    "<button data-test=\"editor-save\" @click=\"$emit('save', { type: 'object', properties: { motd: { type: 'string' } } })\">Save</button>",
})

const QToggleStub = defineComponent({
  name: 'QToggle',
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ['update:modelValue'],
  template:
    '<button data-test="generate-toggle" @click="$emit(\'update:modelValue\', !modelValue)">{{ modelValue }}</button>',
})

describe('GameConfigSchema', () => {
  beforeEach(() => {
    mocks.getGameConfigSchemas.mockReset()
    mocks.updateGameConfigSchemas.mockReset()
    mocks.back.mockReset()
    mocks.notify.mockReset()
  })

  it('saves generate_before_start changes from the config-schema page', async () => {
    mocks.getGameConfigSchemas.mockResolvedValue({
      configSchemasJson: JSON.stringify([
        {
          path: 'server.properties',
          format: 'properties',
          category: 'Core',
          generate_before_start: false,
          schema: {
            type: 'object',
            properties: {},
          },
        },
      ]),
    })
    mocks.updateGameConfigSchemas.mockResolvedValue({
      success: true,
      validationErrors: [],
    })

    const wrapper = mount(GameConfigSchema, {
      global: {
        stubs: {
          ConfigSchemaEditor: ConfigSchemaEditorStub,
          'q-spinner-dots': true,
          'q-toggle': QToggleStub,
          'q-icon': true,
          'q-tooltip': true,
        },
      },
    })

    await flushPromises()

    await wrapper.get('[data-test="generate-toggle"]').trigger('click')
    await wrapper.get('[data-test="editor-save"]').trigger('click')
    await flushPromises()

    expect(mocks.updateGameConfigSchemas).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameConfigSchemas.mock.calls[0][0].configSchemasJson).toContain(
      '"generate_before_start":true',
    )
  })
})
