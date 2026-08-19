import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'

import { ConfigFieldDataSchema } from '@/proto/xylona_pb'

import ConfigFileEditor from './ConfigFileEditor.vue'

const ButtonStub = defineComponent({
  emits: ['click'],
  template: '<button v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></button>',
})

const ToggleStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ['update:modelValue'],
  template:
    '<button class="toggle-stub" type="button" @click="$emit(\'update:modelValue\', !modelValue)" />',
})

const InputStub = defineComponent({
  inheritAttrs: false,
  template: '<input />',
})

describe('ConfigFileEditor', () => {
  it('saves only edited fields', async () => {
    const wrapper = mount(ConfigFileEditor, {
      props: {
        filePath: 'serverconfig.xml',
        format: 'xml',
        category: 'Server',
        categoryColor: '',
        fields: [
          create(ConfigFieldDataSchema, {
            key: 'EACEnabled',
            value: 'true',
            fieldType: 'boolean',
            group: 'Technical',
          }),
          create(ConfigFieldDataSchema, {
            key: 'SandboxCode',
            value: 'AAAJABJACJADJARFBNC',
            fieldType: 'string',
            group: 'Game Rules',
            isMissingFromFile: true,
          }),
        ],
        advancedFields: [],
        validationErrors: [],
        isMissing: false,
        saving: false,
        generating: false,
      },
      global: {
        stubs: {
          QBadge: true,
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

    await wrapper.find('[data-test="config-row-EACEnabled"] .toggle-stub').trigger('click')
    await wrapper.find('.save-btn').trigger('click')

    const emitted = wrapper.emitted('save')
    expect(emitted).toHaveLength(1)
    expect([...(emitted?.[0]?.[0] as Map<string, string>)]).toEqual([['EACEnabled', 'false']])
  })
})
