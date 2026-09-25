import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { ConfigSchemaEntry } from './config-schema-types'
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
  it('turns the empty state into a quick onboarding checklist', () => {
    const wrapper = mount(ConfigSchemaList, {
      props: {
        modelValue: [],
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

    expect(wrapper.text()).toContain('No configuration files defined yet.')
    expect(wrapper.text()).toContain('Start with the file operators edit most often.')
    expect(wrapper.text()).toContain('Add the file path you want Xylona to manage.')
    expect(wrapper.text()).toContain(
      'Open the schema editor to mark managed fields, validation, and defaults.',
    )
  })

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
    expect(payload[0]?.generate_before_start).toBe(true)
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
    expect(buttons[0]?.attributes('aria-label')).toBe('Generate server.properties before start')
    expect(buttons[0]?.attributes('aria-pressed')).toBe('false')
    expect(buttons[1]?.attributes('aria-label')).toBe('Edit schema for server.properties')
    expect(buttons[2]?.attributes('aria-label')).toBe('Remove server.properties')
  })

  it('provides an explicit format edit trigger without relying on hover', () => {
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

    const formatEditButton = wrapper.get('[data-testid="config-schema-format-edit-0"]')

    expect(formatEditButton.attributes('aria-label')).toBe(
      'Edit format of server.properties, currently properties',
    )
    expect(formatEditButton.text()).toContain('properties')
    expect(formatEditButton.text()).toContain('Edit format')
  })

  it('renders the format button in the row while the popup is closed', () => {
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
          // A closed q-popup-edit renders none of its slot content.
          'q-popup-edit': true,
          'q-select': true,
          'q-tooltip': true,
          AddConfigFileDialog: true,
        },
      },
    })

    const formatEditButton = wrapper.get('[data-testid="config-schema-format-edit-0"]')

    // The popup opens on a click of its parent, so it must sit inside the button.
    expect(formatEditButton.find('q-popup-edit-stub').exists()).toBe(true)
    expect(wrapper.find('q-select-stub').exists()).toBe(false)
  })

  it("keeps a removed file's slot as a focused undo row until Undo restores it", async () => {
    const entry = (path: string, category: string): ConfigSchemaEntry => ({
      path,
      format: 'properties',
      category,
      generate_before_start: false,
      schema: { type: 'object', properties: {} },
    })
    const server = entry('server.properties', 'Core')
    const ops = entry('ops.json', 'Access')
    const wrapper = mount(ConfigSchemaList, {
      attachTo: document.body,
      props: {
        modelValue: [server, ops],
        'onUpdate:modelValue': (next: ConfigSchemaEntry[]) => {
          void wrapper.setProps({ modelValue: next })
        },
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
          'q-popup-edit': true,
          'q-select': true,
          'q-tooltip': true,
          AddConfigFileDialog: true,
        },
      },
    })

    await wrapper.get('[aria-label="Remove server.properties"]').trigger('click')
    expect(wrapper.props('modelValue')).toEqual([ops])

    // The last Core file went, but its category stays in place around the undo row.
    const categories = wrapper.findAll('.schema-category')
    expect(categories.map((category) => category.get('.category-label').text())).toEqual([
      'Core',
      'Access',
    ])
    expect(categories[0]?.get('[role="status"]').text()).toContain('Removed server.properties.')
    expect(document.activeElement?.getAttribute('aria-label')).toBe(
      'Undo removing server.properties',
    )

    await wrapper.get('[aria-label="Undo removing server.properties"]').trigger('click')
    expect(wrapper.props('modelValue')).toEqual([server, ops])
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(document.activeElement).toBe(
      wrapper.get('[data-testid="config-schema-format-edit-0"]').element,
    )
    wrapper.unmount()
  })

  it('disables schema editing until the game has been saved', () => {
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
        canEditSchemas: false,
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
    expect(buttons[1]?.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Save changes to edit schemas.')
  })
})
