import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GameConfigSchema from './GameConfigSchema.vue'

const mocks = vi.hoisted(() => ({
  getGame: vi.fn(),
  getGameConfigSchemas: vi.fn(),
  updateGameConfigSchemas: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
  notifyConnectError: vi.fn(),
  useUnsavedChangesGuard: vi.fn(),
  fileIndex: '0',
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({
      params: {
        id: 'minecraft',
        fileIndex: mocks.fileIndex,
      },
    }),
  }
})

vi.mock('@/utils/unsaved-changes-guard', () => ({
  useUnsavedChangesGuard: mocks.useUnsavedChangesGuard,
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
  notifyConnectError: mocks.notifyConnectError,
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getGame: mocks.getGame,
      getGameConfigSchemas: mocks.getGameConfigSchemas,
      updateGameConfigSchemas: mocks.updateGameConfigSchemas,
    }),
  }
})

const ConfigSchemaEditorStub = defineComponent({
  name: 'ConfigSchemaEditor',
  setup(_, { expose }) {
    expose({
      isDirty: ref(false),
      buildSchema: () => ({ type: 'object', properties: { motd: { type: 'string' } } }),
    })
    return {}
  },
  template: '<div data-test="editor" />',
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

const QBtnStub = defineComponent({
  name: 'QBtn',
  props: { label: { type: String, default: '' } },
  emits: ['click'],
  template: '<button v-bind="$attrs" @click="$emit(\'click\')">{{ label }}</button>',
})

function mountPage() {
  return mount(GameConfigSchema, {
    global: {
      stubs: {
        ConfigSchemaEditor: ConfigSchemaEditorStub,
        EmptyState: {
          props: ['title', 'description'],
          template:
            '<div data-test="empty-state">{{ title }} {{ description }}<slot name="actions" /></div>',
        },
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        'q-spinner-dots': true,
        'q-banner': { template: '<div v-bind="$attrs"><slot /></div>' },
        'q-toggle': QToggleStub,
        'q-btn': QBtnStub,
        'q-icon': true,
      },
    },
  })
}

describe('GameConfigSchema', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.fileIndex = '0'
    mocks.getGame.mockResolvedValue({ game: { id: 'minecraft', name: 'Minecraft' } })
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
  })

  it('names the game and file, and saves the schema with the file behavior', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('nav').text()).toContain('Minecraft')
    expect(wrapper.get('nav').text()).toContain('server.properties')

    await wrapper.get('[data-test="generate-toggle"]').trigger('click')
    await wrapper.get('[data-test="save-schema"]').trigger('click')
    await flushPromises()

    expect(mocks.updateGameConfigSchemas).toHaveBeenCalledTimes(1)
    const saved = mocks.updateGameConfigSchemas.mock.calls[0]?.[0].configSchemasJson
    expect(saved).toContain('"generate_before_start":true')
    expect(saved).toContain('"motd"')
    expect(mocks.notifySuccess).toHaveBeenCalledWith('Schema saved successfully')
  })

  it('keeps server validation errors on screen until the next successful save', async () => {
    mocks.updateGameConfigSchemas.mockResolvedValueOnce({
      success: false,
      validationErrors: ['motd: default must be a string'],
    })
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-test="save-schema"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="save-errors"]').text()).toContain(
      'motd: default must be a string',
    )

    await wrapper.get('[data-test="save-schema"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="save-errors"]').exists()).toBe(false)
  })

  it('shows a way back instead of a blank editor for a missing file', async () => {
    mocks.fileIndex = '5'
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="empty-state"]').text()).toContain('Config file not found')
    expect(wrapper.find('[data-test="editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="save-schema"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/games/minecraft/edit"]').exists()).toBe(true)
  })

  it('treats a missing file behavior as off, so toggling it back is not an unsaved change', async () => {
    mocks.getGameConfigSchemas.mockResolvedValue({
      configSchemasJson: JSON.stringify([
        { path: 'serverconfig.xml', format: 'xml', category: 'Core', schema: { type: 'object' } },
      ]),
    })
    const wrapper = mountPage()
    await flushPromises()

    const isDirty = mocks.useUnsavedChangesGuard.mock.calls[0]?.[0] as Ref<boolean>
    const toggle = wrapper.get('[data-test="generate-toggle"]')
    expect(toggle.text()).toBe('false')
    expect(isDirty.value).toBe(false)

    await toggle.trigger('click')
    expect(isDirty.value).toBe(true)

    await toggle.trigger('click')
    expect(isDirty.value).toBe(false)
  })
})
