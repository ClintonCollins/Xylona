import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import {
  AdvancedFieldSchema,
  ConfigFieldDataSchema,
  ConfigValidationErrorSchema,
} from '@/proto/xylona_pb'
import type { AdvancedField, ConfigFieldData, ConfigValidationError } from '@/proto/xylona_pb'

import ConfigAdvancedFields from './ConfigAdvancedFields.vue'
import ConfigFileEditor from './ConfigFileEditor.vue'

const mocks = vi.hoisted(() => {
  const state = { confirm: true }
  return {
    state,
    dialog: vi.fn(() => ({
      onOk(handler: () => void) {
        if (state.confirm) handler()
      },
    })),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return { ...actual, useQuasar: () => ({ dialog: mocks.dialog }) }
})

const ButtonStub = defineComponent({
  props: { label: { type: String, default: '' }, disable: Boolean },
  emits: ['click'],
  template:
    '<button v-bind="$attrs" :disabled="disable" @click="$emit(\'click\', $event)">{{ label }}<slot /></button>',
})

const ToggleStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ['update:modelValue'],
  template:
    '<button class="toggle-stub" type="button" @click="$emit(\'update:modelValue\', !modelValue)" />',
})

const InputStub = defineComponent({
  inheritAttrs: false,
  props: {
    modelValue: { type: [String, Number], default: '' },
    errorMessage: { type: String, default: undefined },
    type: { type: String, default: 'text' },
  },
  emits: ['update:modelValue'],
  template: `<div class="input-stub" :data-error="errorMessage">
      <input :aria-label="$attrs['aria-label']" :autocomplete="$attrs.autocomplete" :type="type"
        :value="modelValue"
        @input="$emit('update:modelValue', $event.target.value)" />
      <slot name="append" />
    </div>`,
})

function field(overrides: Partial<ConfigFieldData>): ConfigFieldData {
  return create(ConfigFieldDataSchema, { fieldType: 'string', group: 'General', ...overrides })
}

function mountEditor(
  props: {
    fields?: ConfigFieldData[]
    advancedFields?: AdvancedField[]
    validationErrors?: ConfigValidationError[]
  } = {},
) {
  return mount(ConfigFileEditor, {
    props: {
      filePath: 'server.properties',
      format: 'properties',
      category: 'Server',
      categoryColor: '',
      fields: props.fields ?? [],
      advancedFields: props.advancedFields ?? [],
      validationErrors: props.validationErrors ?? [],
      isMissing: false,
      saving: false,
      generating: false,
    },
    global: {
      stubs: {
        ConfigAdvancedFields: true,
        QBadge: true,
        QBanner: { template: '<div><slot /></div>' },
        QBtn: ButtonStub,
        QIcon: true,
        QInput: InputStub,
        QSelect: true,
        QSeparator: true,
        QTooltip: true,
        QToggle: ToggleStub,
      },
    },
  })
}

type EditorWrapper = ReturnType<typeof mountEditor>

async function typeInto(wrapper: EditorWrapper, key: string, value: string) {
  await wrapper.find(`[data-test="config-row-${key}"] input`).setValue(value)
}

function saveButton(wrapper: EditorWrapper) {
  return wrapper.find('.save-btn')
}

function savedMaps(wrapper: EditorWrapper): [string, string][][] {
  return (wrapper.emitted('save') ?? []).map((args) => [...(args[0] as Map<string, string>)])
}

describe('ConfigFileEditor', () => {
  it('saves only edited fields', async () => {
    const wrapper = mountEditor({
      fields: [
        field({ key: 'EACEnabled', value: 'true', fieldType: 'boolean', group: 'Technical' }),
        field({
          key: 'SandboxCode',
          value: 'AAAJABJACJADJARFBNC',
          group: 'Game Rules',
          isMissingFromFile: true,
        }),
      ],
    })

    await wrapper.find('[data-test="config-row-EACEnabled"] .toggle-stub').trigger('click')
    await saveButton(wrapper).trigger('click')

    expect(savedMaps(wrapper)).toEqual([[['EACEnabled', 'false']]])
  })

  it('keeps edits after a save until the parent confirms it', async () => {
    const wrapper = mountEditor({ fields: [field({ key: 'motd', value: 'Hello' })] })

    await typeInto(wrapper, 'motd', 'Welcome')
    await saveButton(wrapper).trigger('click')

    // The save is only a request: the edit and Discard stay until confirmed.
    expect(wrapper.find('[data-test="config-row-motd"]').classes()).toContain('setting-edited')
    expect(wrapper.find('.discard-btn').exists()).toBe(true)

    ;(wrapper.vm as unknown as { confirmSaved: () => void }).confirmSaved()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-test="config-row-motd"]').classes()).not.toContain('setting-edited')
    expect(saveButton(wrapper).text()).toContain('Saved')
  })

  it('shows server errors on the row until the value changes again', async () => {
    const wrapper = mountEditor({ fields: [field({ key: 'motd', title: 'MOTD', value: 'Hi' })] })

    await typeInto(wrapper, 'motd', 'bad value')
    await saveButton(wrapper).trigger('click')
    await wrapper.setProps({
      validationErrors: [
        create(ConfigValidationErrorSchema, { field: 'motd', message: 'Not allowed' }),
      ],
    })

    const row = wrapper.find('[data-test="config-row-motd"]')
    expect(row.classes()).toContain('setting-invalid')
    expect(row.find('.input-stub').attributes('data-error')).toBe('Not allowed')
    expect(wrapper.find('.validation-banner').text()).toContain('MOTD: Not allowed')
    expect(
      wrapper.find<HTMLInputElement>('[data-test="config-row-motd"] input').element.value,
    ).toBe('bad value')

    await typeInto(wrapper, 'motd', 'better value')

    expect(wrapper.find('[data-test="config-row-motd"]').classes()).not.toContain('setting-invalid')
  })

  it('blocks Save while an edited value breaks its limits', async () => {
    const wrapper = mountEditor({
      fields: [
        field({
          key: 'max-players',
          fieldType: 'integer',
          value: '20',
          minimum: 1n,
          maximum: 100n,
        }),
      ],
    })

    await typeInto(wrapper, 'max-players', '500')
    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()
    await saveButton(wrapper).trigger('click')
    expect(wrapper.emitted('save')).toBeUndefined()

    await typeInto(wrapper, 'max-players', '50')
    expect(saveButton(wrapper).attributes('disabled')).toBeUndefined()
  })

  it('discards edits once confirmed and tells the parent', async () => {
    const wrapper = mountEditor({ fields: [field({ key: 'motd', value: 'Hello' })] })
    expect(wrapper.find('.discard-btn').exists()).toBe(false)

    await typeInto(wrapper, 'motd', 'Welcome')
    mocks.state.confirm = false
    await wrapper.find('.discard-btn').trigger('click')

    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Discard changes to server.properties?', focus: 'cancel' }),
    )
    expect(wrapper.emitted('discard')).toBeUndefined()
    expect(
      wrapper.find<HTMLInputElement>('[data-test="config-row-motd"] input').element.value,
    ).toBe('Welcome')

    mocks.state.confirm = true
    await wrapper.find('.discard-btn').trigger('click')

    expect(wrapper.emitted('discard')).toHaveLength(1)
    expect(
      wrapper.find<HTMLInputElement>('[data-test="config-row-motd"] input').element.value,
    ).toBe('Hello')
    expect(wrapper.find('.discard-btn').exists()).toBe(false)
  })

  it('shows managed values as operators read them', () => {
    const wrapper = mountEditor({
      fields: [
        field({
          key: 'enable-rcon',
          fieldType: 'boolean',
          value: 'true',
          isManaged: true,
          managedSource: 'xylona.local_console_enabled',
        }),
        field({
          key: 'rcon.port',
          fieldType: 'integer',
          value: '25577',
          isManaged: true,
          managedSource: 'xylona.local_console_port',
        }),
        field({
          key: 'rcon.password',
          value: '',
          isManaged: true,
          managedSource: 'xylona.local_console_password',
        }),
      ],
    })

    const managed = (key: string) => wrapper.find(`[data-test="config-row-${key}"]`)
    expect(managed('enable-rcon').find('[data-test="managed-value"]').text()).toBe('Enabled')
    expect(managed('rcon.port').find('[data-test="managed-value"]').text()).toBe('25577')
    expect(managed('rcon.port').text()).toContain('Source: Local Console Port')
    expect(managed('rcon.password').find('[data-test="managed-value"]').text()).toBe('••••••••')
  })

  it('masks secret values until revealed', async () => {
    const wrapper = mountEditor({ fields: [field({ key: 'ServerPassword', value: 'hunter2' })] })

    const input = () => wrapper.find('[data-test="config-row-ServerPassword"] input')
    expect(input().attributes('type')).toBe('password')
    // Keeps password managers from saving or autofilling game secrets.
    expect(input().attributes('autocomplete')).toBe('new-password')

    await wrapper.find('[data-test="config-row-ServerPassword"] button').trigger('click')

    expect(input().attributes('type')).toBe('text')
  })

  it('masks secret advanced values without looking like a login', () => {
    const wrapper = mount(ConfigAdvancedFields, {
      props: {
        fields: [create(AdvancedFieldSchema, { key: 'management-server-secret', value: 's3cret' })],
      },
      global: {
        stubs: {
          QBanner: { template: '<div><slot /></div>' },
          QBtn: ButtonStub,
          QExpansionItem: { template: '<div><slot /></div>' },
          QIcon: true,
          QInput: InputStub,
          QTooltip: true,
        },
      },
    })

    const inputs = wrapper.findAll('[data-test="advanced-row-management-server-secret"] input')
    expect(inputs.map((i) => i.attributes('autocomplete'))).toEqual(['off', 'new-password'])
    expect(inputs[1]?.attributes('type')).toBe('password')
  })

  it('shows numbers without the zero padding and ignores equal numbers', async () => {
    const wrapper = mountEditor({
      fields: [field({ key: 'ExpRate', fieldType: 'number', value: '1.000000' })],
    })

    expect(
      wrapper.find<HTMLInputElement>('[data-test="config-row-ExpRate"] input').element.value,
    ).toBe('1')

    await typeInto(wrapper, 'ExpRate', '1.0')
    expect(wrapper.find('[data-test="config-row-ExpRate"]').classes()).not.toContain(
      'setting-edited',
    )
  })

  it('points a search that only matches advanced fields at them', async () => {
    const wrapper = mountEditor({
      fields: [field({ key: 'motd', value: 'Hello' })],
      advancedFields: [create(AdvancedFieldSchema, { key: 'difficulty', value: 'easy' })],
    })

    await wrapper.find('input[aria-label="Search configuration fields"]').setValue('difficulty')

    expect(wrapper.text()).toContain('No schema settings match')
    expect(wrapper.find('[data-test="advanced-match-count"]').text()).toBe(
      '1 match in Advanced Fields below',
    )
  })
})
