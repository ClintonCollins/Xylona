import { defineComponent, nextTick } from 'vue'
import { shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ConfigSchemaEditor from './ConfigSchemaEditor.vue'

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
})
