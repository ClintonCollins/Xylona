import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ConfigSchemaList from './ConfigSchemaList.vue'

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  inheritAttrs: false,
  props: {
    disable: {
      type: Boolean,
      default: false,
    },
    icon: {
      type: String,
      default: '',
    },
    label: {
      type: String,
      default: '',
    },
  },
  emits: ['click'],
  template:
    '<button v-bind="$attrs" :data-icon="icon" :data-label="label" :disabled="disable" @click="$emit(\'click\')"><slot /></button>',
})

describe('ConfigSchemaList', () => {
  it('toggles generate-before-start for an existing schema entry', async () => {
    const wrapper = mount(ConfigSchemaList, {
      props: {
        modelValue: [
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
        ],
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          'q-list': { template: '<div><slot /></div>' },
          'q-item': { template: '<div><slot /></div>' },
          'q-item-section': { template: '<div><slot /></div>' },
          'q-item-label': { template: '<div><slot /></div>' },
          'q-icon': true,
          'q-badge': { template: '<span><slot />{{ label }}</span>', props: ['label'] },
          'q-popup-edit': { template: '<div><slot :value="modelValue" :set="() => {}" /></div>' },
          'q-select': true,
          'q-tooltip': true,
          AddConfigFileDialog: true,
        },
      },
    })

    await wrapper.get('[data-test="toggle-generate-before-start"]').trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()

    const payload = emitted?.[0]?.[0] as Array<{ generate_before_start: boolean }>
    expect(payload).toHaveLength(1)
    expect(payload[0].generate_before_start).toBe(true)
  })

  it('labels icon-only row actions for assistive technology', () => {
    const wrapper = mount(ConfigSchemaList, {
      props: {
        modelValue: [
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
        ],
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          'q-list': { template: '<div><slot /></div>' },
          'q-item': { template: '<div><slot /></div>' },
          'q-item-section': { template: '<div><slot /></div>' },
          'q-item-label': { template: '<div><slot /></div>' },
          'q-icon': true,
          'q-badge': { template: '<span><slot />{{ label }}</span>', props: ['label'] },
          'q-popup-edit': { template: '<div><slot :value="modelValue" :set="() => {}" /></div>' },
          'q-select': true,
          'q-tooltip': true,
          AddConfigFileDialog: true,
        },
      },
    })

    const buttons = wrapper.findAll('.schema-file-actions button')
    expect(buttons).toHaveLength(3)
    expect(buttons[0].attributes('aria-label')).toBe('Toggle generate before start')
    expect(buttons[1].attributes('aria-label')).toBe('Edit schema')
    expect(buttons[2].attributes('aria-label')).toBe('Remove schema')
  })
})
