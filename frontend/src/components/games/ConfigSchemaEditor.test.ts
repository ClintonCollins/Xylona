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
        filePath: 'server.properties',
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
        filePath: 'server.properties',
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

    await wrapper.get('[data-label="Save Schema"]').trigger('click')

    const emitted = wrapper.emitted('save')
    expect(emitted).toBeTruthy()

    const savedSchema = emitted?.[0]?.[0] as {
      properties: Record<string, { 'x-managed'?: { source: string } }>
    }

    expect(savedSchema.properties['server-port']?.['x-managed']?.source).toBe('game_server.port')
  })
})
