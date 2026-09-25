import { defineComponent, nextTick, onMounted } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import ConfigSchemaEditor from './ConfigSchemaEditor.vue'
import RemovedItemUndo from './RemovedItemUndo.vue'

const mocks = vi.hoisted(() => {
  // A stand-in Monaco editor: the test edits `value` and fires the change listener.
  const editor = {
    value: '',
    onChange: () => {},
    getValue: () => editor.value,
    onDidChangeModelContent: (listener: () => void) => {
      editor.onChange = listener
    },
    dispose: vi.fn(),
  }
  return {
    editor,
    notifyError: vi.fn(),
    monaco: {
      editor: {
        create: (_container: unknown, options: { value: string }) => {
          editor.value = options.value
          return editor
        },
      },
    },
  }
})

vi.mock('@/components/editor/monaco-runtime', () => ({
  loadMonacoRuntime: async () => mocks.monaco,
}))

vi.mock('@/api/notifications', () => ({
  notifyError: mocks.notifyError,
  notifySuccess: vi.fn(),
}))

const QBtnToggleStub = defineComponent({
  props: {
    options: { type: Array as () => { label: string; value: string }[], default: () => [] },
  },
  emits: ['update:modelValue'],
  template:
    '<div><button v-for="option in options" :key="option.value" :data-mode="option.value" @click="$emit(\'update:modelValue\', option.value)">{{ option.label }}</button></div>',
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  emits: ['click'],
  template: '<button :data-label="label" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const ConfigSchemaFieldCardStub = defineComponent({
  name: 'ConfigSchemaFieldCardStub',
  props: {
    modelValue: {
      type: Object,
      required: true,
    },
  },
  template: '<div data-testid="field-card" :data-managed-source="modelValue.managedSource"></div>',
})

describe('ConfigSchemaEditor managed sources', () => {
  it('maps backend managed source keys to the frontend option values', async () => {
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: {
        schema: {
          type: 'object',
          properties: {
            'server-port': {
              type: 'integer',
              'x-managed': { source: 'game_server.port' },
            },
          },
        },
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          ConfigSchemaFieldCard: ConfigSchemaFieldCardStub,
          'q-btn-toggle': true,
          'q-separator': true,
          'q-checkbox': true,
          'q-input': true,
          'q-icon': true,
          'q-dialog': true,
          'q-card': true,
          'q-card-section': true,
          'q-card-actions': true,
          'q-item': true,
          'q-item-section': true,
          'q-item-label': true,
          'q-list': true,
          'q-menu': true,
          'q-tooltip': true,
          'q-slide-transition': true,
          ConfigImportInput: true,
        },
      },
    })

    await nextTick()

    expect(wrapper.get('[data-testid="field-card"]').attributes('data-managed-source')).toBe(
      'server_port',
    )
  })

  it('converts frontend managed source values back to backend keys when saving', async () => {
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: {
        schema: {
          type: 'object',
          properties: {
            'server-port': {
              type: 'integer',
              'x-managed': { source: 'server_port' },
            },
          },
        },
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          ConfigSchemaFieldCard: ConfigSchemaFieldCardStub,
          'q-btn-toggle': true,
          'q-separator': true,
          'q-checkbox': true,
          'q-input': true,
          'q-icon': true,
          'q-dialog': true,
          'q-card': true,
          'q-card-section': true,
          'q-card-actions': true,
          'q-item': true,
          'q-item-section': true,
          'q-item-label': true,
          'q-list': true,
          'q-menu': true,
          'q-tooltip': true,
          'q-slide-transition': true,
          ConfigImportInput: true,
        },
      },
    })

    await nextTick()
    const savedSchema = wrapper.vm.buildSchema() as {
      properties: Record<string, { 'x-managed'?: { source: string } }>
    }

    expect(savedSchema.properties['server-port']?.['x-managed']?.source).toBe('game_server.port')
  })

  it('tracks unsaved edits against the loaded schema', async () => {
    const RemovableFieldCardStub = defineComponent({
      emits: ['remove'],
      template: '<div data-testid="field-card" @click="$emit(\'remove\')"></div>',
    })
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: {
        schema: {
          type: 'object',
          properties: {
            motd: { type: 'string', title: 'MOTD', default: 'Hello' },
            'max-players': { type: 'integer', default: 20 },
          },
          required: ['motd'],
        },
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          ConfigSchemaFieldCard: RemovableFieldCardStub,
        },
      },
    })

    await nextTick()
    expect(wrapper.vm.isDirty).toBe(false)

    // An unnamed new field is not saved, so it is not an unsaved change yet.
    await wrapper.get('[data-label="Add Field"]').trigger('click')
    expect(wrapper.vm.isDirty).toBe(false)

    await wrapper.get('[data-testid="field-card"]').trigger('click')
    expect(wrapper.vm.isDirty).toBe(true)

    // A newly loaded (or just saved) schema is the new clean baseline.
    await wrapper.setProps({
      schema: { type: 'object', properties: { motd: { type: 'string' } } },
    })
    expect(wrapper.vm.isDirty).toBe(false)
  })

  it("keeps a removed field's slot as an undo row that restores it", async () => {
    const RemovableFieldCardStub = defineComponent({
      emits: ['remove'],
      template: '<div data-testid="field-card" @click="$emit(\'remove\')"></div>',
    })
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: { schema: { type: 'object', properties: { motd: { type: 'string' } } } },
      global: { stubs: { 'q-btn': QBtnStub, ConfigSchemaFieldCard: RemovableFieldCardStub } },
    })
    await nextTick()

    await wrapper.get('[data-testid="field-card"]').trigger('click')
    expect(wrapper.find('[data-testid="field-card"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('No fields defined yet')

    const undo = wrapper.findComponent(RemovedItemUndo)
    expect(undo.props('label')).toBe('motd')
    undo.vm.$emit('undo')
    await nextTick()

    expect(wrapper.find('[data-testid="field-card"]').exists()).toBe(true)
    expect(wrapper.vm.isDirty).toBe(false)
  })

  it('keeps a field card mounted while its key is edited', async () => {
    const mounted = vi.fn()
    const RenamingFieldCardStub = defineComponent({
      props: { modelValue: { type: Object, required: true } },
      emits: ['update:modelValue'],
      setup() {
        onMounted(mounted)
      },
      template:
        '<div data-testid="field-card" @click="$emit(\'update:modelValue\', { ...modelValue, key: modelValue.key + \'x\' })">{{ modelValue.key }}</div>',
    })
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: { schema: { type: 'object', properties: { motd: { type: 'string' } } } },
      global: { stubs: { 'q-btn': QBtnStub, ConfigSchemaFieldCard: RenamingFieldCardStub } },
    })
    await nextTick()
    expect(mounted).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="field-card"]').trigger('click')
    await nextTick()

    expect(wrapper.get('[data-testid="field-card"]').text()).toBe('motdx')
    expect(mounted).toHaveBeenCalledTimes(1)
  })

  it('stays in Raw JSON with the edits when the JSON is invalid', async () => {
    const wrapper = shallowMount(ConfigSchemaEditor, {
      props: { schema: { type: 'object', properties: { motd: { type: 'string' } } } },
      global: { stubs: { 'q-btn-toggle': QBtnToggleStub } },
    })
    await nextTick()

    await wrapper.get('[data-mode="json"]').trigger('click')
    await flushPromises()
    mocks.editor.value = '{ "type": "object", "properties": {'
    mocks.editor.onChange()

    await wrapper.get('[data-mode="form"]').trigger('click')
    await flushPromises()

    expect(mocks.notifyError).toHaveBeenCalledWith(
      'Fix the JSON errors before switching to the form.',
    )
    expect(wrapper.find('.json-editor').exists()).toBe(true)
    expect(mocks.editor.dispose).not.toHaveBeenCalled()
    expect(wrapper.vm.isDirty).toBe(true)

    // Valid JSON switches back and becomes the form.
    mocks.editor.value =
      '{ "type": "object", "properties": { "level-name": { "type": "string" } } }'
    mocks.editor.onChange()
    await wrapper.get('[data-mode="form"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.json-editor').exists()).toBe(false)
    expect(mocks.editor.dispose).toHaveBeenCalledTimes(1)
    expect(Object.keys(wrapper.vm.buildSchema()?.properties ?? {})).toEqual(['level-name'])
  })
})
