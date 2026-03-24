import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ConfigFileEditor from './ConfigFileEditor.vue'

describe('ConfigFileEditor', () => {
  it('renders managed fields as locked, non-editable values', () => {
    const wrapper = mount(ConfigFileEditor, {
      props: {
        filePath: 'server.properties',
        format: 'properties',
        category: 'Core',
        categoryColor: '#3B82F6',
        fields: [
          {
            key: 'server-port',
            value: '25565',
            title: 'Server Port',
            description: 'Port exposed by the server',
            fieldType: 'integer',
            defaultValue: '25565',
            isManaged: true,
            managedSource: 'game_server.port',
            isMissingFromFile: false,
            enumOptions: [],
            enumLabels: [],
            required: false,
            allowMultiple: false,
            values: [],
            group: '',
          },
        ],
        advancedFields: [],
        validationErrors: [],
        isMissing: false,
        saving: false,
        generating: false,
      },
      global: {
        stubs: {
          'q-badge': { template: '<span><slot /></span>' },
          'q-banner': { template: '<div><slot /></div>' },
          'q-btn': { template: '<button><slot /></button>' },
          'q-icon': { template: '<i><slot /></i>' },
          'q-input': { template: '<input />' },
          'q-select': { template: '<select />' },
          'q-separator': true,
          'q-toggle': { template: '<button />' },
          'q-tooltip': { template: '<span><slot /></span>' },
          ConfigAdvancedFields: true,
        },
      },
    })

    const managedRow = wrapper.get('[data-test="config-row-server-port"]')

    expect(managedRow.text()).toContain('Managed by server settings')
    expect(managedRow.find('[data-test="managed-field-display"]').exists()).toBe(true)
    expect(managedRow.find('input').exists()).toBe(false)
    expect(managedRow.find('select').exists()).toBe(false)
    expect(managedRow.find('button').exists()).toBe(false)
  })
})
